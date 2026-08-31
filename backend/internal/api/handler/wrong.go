package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/review"
)

// WrongHandler 错题本 / 复习接口。
type WrongHandler struct {
	repo      *sqlite.Repository
	scheduler review.ReviewScheduler
}

func NewWrongHandler(repo *sqlite.Repository, scheduler review.ReviewScheduler) *WrongHandler {
	return &WrongHandler{repo: repo, scheduler: scheduler}
}

func (h *WrongHandler) List(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	f := sqlite.WrongFilter{
		UserID:   uid,
		Page:     parseInt(c.Query("page"), 1),
		PageSize: parseInt(c.Query("page_size"), 20),
	}
	if v := c.Query("subject_id"); v != "" {
		id := parseID(v)
		f.SubjectID = &id
	}
	if v := c.Query("chapter_id"); v != "" {
		id := parseID(v)
		f.ChapterID = &id
	}
	if v := c.Query("status"); v != "" {
		f.Status = v
	}
	if v := c.Query("due"); v == "true" {
		f.DueOnly = true
	}
	if v := c.Query("min_wrong_count"); v != "" {
		f.MinWrongCount = parseInt(v, 0)
	}
	if v := c.Query("sort"); v == "priority" {
		f.SortByPriority = true
	}
	list, total, err := h.repo.ListWrongQuestions(c.Request.Context(), f)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询错题失败")
		return
	}
	api.PageOK(c, list, total, f.Page, f.PageSize)
}

func (h *WrongHandler) Due(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	f := sqlite.WrongFilter{UserID: uid, DueOnly: true, SortByDue: true, Page: 1, PageSize: 100}
	list, _, err := h.repo.ListWrongQuestions(c.Request.Context(), f)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询待复习失败")
		return
	}
	api.OK(c, list)
}

// AddManual 手动加入错题本。
func (h *WrongHandler) AddManual(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	qid := parseID(c.Param("id"))
	w, err := h.repo.UpsertWrongQuestion(c.Request.Context(), uid, qid)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "加入错题本失败")
		return
	}
	api.OK(c, w)
}

// Remove 移除错题。
func (h *WrongHandler) Remove(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	qid := parseID(c.Param("id"))
	if err := h.repo.RemoveWrongQuestion(c.Request.Context(), uid, qid); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "移除失败")
		return
	}
	api.OK(c, gin.H{"removed": true})
}

// ReviewSubmit 提交复习结果（again/hard/good/easy）。
func (h *WrongHandler) ReviewSubmit(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	wqID := parseID(c.Param("id"))
	var req struct {
		Result string `json:"result" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	switch req.Result {
	case domain.ReviewResultAgain, domain.ReviewResultHard, domain.ReviewResultGood, domain.ReviewResultEasy:
	default:
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "result 必须是 again/hard/good/easy")
		return
	}

	w, err := h.repo.GetWrongQuestion(c.Request.Context(), wqID)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "错题不存在")
		return
	}
	if w.UserID != uid {
		api.Fail(c, http.StatusForbidden, api.ErrUnauthorized, "无权访问")
		return
	}

	input := review.ReviewInput{
		WrongCount:    w.WrongCount,
		CorrectCount:  w.CorrectCount,
		ReviewCount:   w.ReviewCount,
		MasteryScore:  w.MasteryScore,
		IntervalDays:  w.IntervalDays,
		Difficulty:    3,
		Result:        req.Result,
	}
	if w.LastReviewAt != nil {
		input.LastReviewAt = *w.LastReviewAt
	}
	if w.LastWrongAt != nil {
		input.LastWrongAt = *w.LastWrongAt
	}
	// 题目难度
	if q, err := h.repo.GetQuestion(c.Request.Context(), w.QuestionID); err == nil {
		input.Difficulty = q.Difficulty
	}

	r, err := h.scheduler.Calculate(c.Request.Context(), input)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "计算复习计划失败")
		return
	}

	isCorrect := req.Result != domain.ReviewResultAgain
	if isCorrect {
		w.CorrectCount++
	} else {
		w.WrongCount++
		w.IntervalDays = 0
		w.MasteryScore = r.MasteryScore
	}
	w.ReviewCount++
	w.MasteryScore = r.MasteryScore
	w.IntervalDays = r.IntervalDays
	w.PriorityScore = r.PriorityScore
	w.NextReviewAt = &r.NextReviewAt
	w.Status = r.Status
	if err := h.repo.UpdateWrongQuestion(c.Request.Context(), w); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新错题失败")
		return
	}
	// 复习记录 + 统计
	_ = h.repo.RecordReview(c.Request.Context(), uid, wqID, req.Result, isCorrect, r.IntervalDays)
	date := time.Now().UTC().Format("2006-01-02")
	_ = h.repo.UpsertDailyStat(c.Request.Context(), uid, date, 0, 0, 1, 0)

	api.OK(c, gin.H{
		"wrong_question": w,
		"review_result":  r,
		"next_review_at": r.NextReviewAt,
	})
}

// Summary 错题本统计。
func (h *WrongHandler) Summary(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	var total, due, mastered int64
	_ = h.repo.DB().QueryRowContext(c.Request.Context(),
		`SELECT COUNT(*) FROM wrong_questions WHERE user_id = ?`, uid).Scan(&total)
	_ = h.repo.DB().QueryRowContext(c.Request.Context(),
		`SELECT COUNT(*) FROM wrong_questions WHERE user_id = ? AND next_review_at IS NOT NULL AND next_review_at <= datetime('now')`, uid).Scan(&due)
	_ = h.repo.DB().QueryRowContext(c.Request.Context(),
		`SELECT COUNT(*) FROM wrong_questions WHERE user_id = ? AND status = 'mastered'`, uid).Scan(&mastered)
	api.OK(c, gin.H{"total": total, "due": due, "mastered": mastered})
}

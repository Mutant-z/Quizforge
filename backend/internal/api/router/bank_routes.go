package router

import (
	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api/handler"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/review"
	"github.com/quiztrace/quiztrace/internal/service"
)

func registerBankRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	svc := service.NewQuestionService(repo)
	h := handler.NewBankHandler(svc, repo)

	banks := g.Group("/question-banks", middleware.Auth(deps.Token))
	banks.GET("", h.List)
	banks.GET("/:id", h.Get)
	banks.POST("", h.Create)
	banks.DELETE("/:id", h.Delete)
	banks.GET("/:id/subjects", h.ListSubjects)
	banks.POST("/:id/subjects", h.CreateSubject)
	banks.GET("/:id/chapters", h.ListBankChapters)

	subjects := g.Group("/subjects", middleware.Auth(deps.Token))
	subjects.GET("/:subject_id", h.GetSubject)
	subjects.GET("/:subject_id/chapters", h.ListChapters)
	subjects.POST("/:subject_id/chapters", h.CreateChapter)

	chapters := g.Group("/chapters", middleware.Auth(deps.Token))
	chapters.PUT("/:id", h.UpdateChapter)
	chapters.DELETE("/:id", h.DeleteChapter)
	chapters.GET("/:id/questions", h.GetChapterQuestions)
}

func registerQuestionRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	svc := service.NewQuestionService(repo)
	h := handler.NewQuestionHandler(repo, svc)

	q := g.Group("/questions", middleware.Auth(deps.Token))
	q.GET("/search", h.Search)
	q.PUT("/:id", h.Update)
	q.GET("/:id", h.Get)
	q.POST("/:id/favorite", h.ToggleFavorite)
	q.GET("/:id/note", h.GetNote)
	q.PUT("/:id/note", h.PutNote)
	q.DELETE("/:id/note", h.DeleteNote)
}

func registerPracticeRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	scheduler, _ := review.GetScheduler(cfg.Review.Scheduler)
	svc := service.NewPracticeService(repo, scheduler)
	h := handler.NewPracticeHandler(svc, repo)

	p := g.Group("/practice", middleware.Auth(deps.Token))
	p.POST("/preview", h.Preview)
	p.POST("/sessions", h.CreateSession)
	p.POST("/answer", h.AnswerStandalone)
	p.GET("/sessions/:id", h.GetSession)
	p.GET("/sessions/:id/current-question", h.GetCurrentQuestion)
	p.POST("/sessions/:id/answer", h.Answer)
	p.POST("/sessions/:id/next", h.Next)
	p.POST("/sessions/:id/finish", h.Finish)
}

func registerWrongRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	scheduler, _ := review.GetScheduler(cfg.Review.Scheduler)
	h := handler.NewWrongHandler(repo, scheduler)

	w := g.Group("/wrong-questions", middleware.Auth(deps.Token))
	w.GET("", h.List)
	w.GET("/due", h.Due)
	w.GET("/summary", h.Summary)
	w.POST("/:id/review-result", h.ReviewSubmit)
	w.GET("/questions/:id/add", h.AddManual)
	w.DELETE("/questions/:id", h.Remove)
}

func registerReviewRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	// 复习配置（用户偏好）
	repo := sqlite.NewRepository(deps.DB)
	h := handler.NewReviewConfigHandler(repo)
	rg := g.Group("/reviews", middleware.Auth(deps.Token))
	rg.GET("/config", h.GetConfig)
	rg.PUT("/config", h.UpdateConfig)
}

func registerStatRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	svc := service.NewStatService(repo)
	h := handler.NewStatHandler(svc)

	s := g.Group("/statistics", middleware.Auth(deps.Token))
	s.GET("/overview", h.Overview)
	s.GET("/trend", h.Trend)
	s.GET("/chapter-mastery", h.ChapterMastery)
	s.GET("/subject-mastery", h.SubjectMastery)
	s.GET("/wrong-trend", h.WrongTrend)
	s.GET("/weak-knowledge-points", h.WeakKnowledgePoints)
}

func parseParamID(s string) int64 {
	var id int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		id = id*10 + int64(ch-'0')
	}
	return id
}

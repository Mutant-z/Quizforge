package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
)

type VisionDraftService struct {
	repo *sqlite.Repository
	cfg  *config.Config
}

func NewVisionDraftService(repo *sqlite.Repository, cfg *config.Config) *VisionDraftService {
	return &VisionDraftService{repo: repo, cfg: cfg}
}

type RepairAction struct {
	Intent       string   `json:"intent"`
	PageRanges   [][]int  `json:"page_ranges"`
	CandidateIDs []int64  `json:"candidate_ids"`
	ProblemIDs   []string `json:"problem_ids"`
	Instruction  string   `json:"instruction"`
}

func (s *VisionDraftService) Preview(ctx context.Context, jobID int64) (*domain.ImportPreview, error) {
	return s.repo.VisionPreview(ctx, jobID)
}
func (s *VisionDraftService) UpdateCandidate(ctx context.Context, jobID, candidateID int64, expectedVersion int, payload map[string]interface{}) (bool, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	return s.repo.UpdateVisionCandidate(ctx, jobID, candidateID, expectedVersion, string(raw))
}
func (s *VisionDraftService) Commit(ctx context.Context, jobID int64, expectedVersion int) (*sqlite.VisionCommitResult, error) {
	result, err := s.repo.CommitVisionDraft(ctx, jobID, expectedVersion)
	if err != nil {
		return nil, err
	}
	for _, questionID := range result.QuestionIDs {
		question, getErr := s.repo.GetQuestion(ctx, questionID)
		if getErr != nil {
			return nil, getErr
		}
		if indexErr := s.repo.BuildQuestionFTS(ctx, question); indexErr != nil {
			return nil, indexErr
		}
	}
	_, _ = s.repo.EnqueueJob(ctx, "embedding", ImportPayload{ImportJobID: jobID}, 0)
	return result, nil
}

func (s *VisionDraftService) HandleMessage(ctx context.Context, job *domain.ImportJob, content string, expectedVersion int) (*RepairAction, error) {
	if expectedVersion != job.DraftVersion {
		return nil, fmt.Errorf("DRAFT_VERSION_CONFLICT")
	}
	userMessage := &domain.ImportMessage{ImportJobID: job.ID, Role: "user", Content: content, Status: "completed", DraftVersionBefore: job.DraftVersion}
	_, _ = s.repo.AddImportMessage(ctx, userMessage)
	action := s.planRepairAction(ctx, job.UserID, content)
	action.Instruction = content
	raw, _ := json.Marshal(action)
	units, err := s.repo.ListAgentUnits(ctx, job.ID, "")
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for _, unit := range units {
		if repairTargetsUnit(action, unit) {
			ids = append(ids, unit.ID)
		}
	}
	reply := "已分析任务状态，没有需要重新执行的单元。"
	if action.Intent == "explain_status" {
		reply = fmt.Sprintf("当前任务状态为 %s：已提取 %d 题，完整 %d 题，阻断项 %d。", job.Status, job.ExtractedCount, job.CompleteCount, job.BlockingIssueCount)
	} else if len(ids) > 0 {
		count, err := s.repo.RetryAgentUnits(ctx, job.ID, ids)
		if err != nil {
			return nil, err
		}
		_, err = s.repo.EnqueueJob(ctx, "import_pdf", ImportPayload{ImportJobID: job.ID}, 10)
		if err != nil {
			return nil, err
		}
		_ = s.repo.UpdateImportStatus(ctx, job.ID, domain.ImportStatusDirectoryBuilding, domain.ImportStatusDirectoryBuilding, 0.1)
		reply = fmt.Sprintf("已创建定向修复任务，将重新执行 %d 个 Agent 单元。", count)
	}
	assistant := &domain.ImportMessage{ImportJobID: job.ID, Role: "assistant", Content: reply, ActionJSON: string(raw), Status: "completed", DraftVersionBefore: job.DraftVersion, DraftVersionAfter: job.DraftVersion}
	_, _ = s.repo.AddImportMessage(ctx, assistant)
	return action, nil
}

func (s *VisionDraftService) planRepairAction(ctx context.Context, userID int64, content string) *RepairAction {
	ctx = observability.WithUserID(ctx, userID)
	registry := provider.NewRegistry(s.repo.DB(), cryptFrom(s.cfg))
	cfg, err := registry.Default(ctx, provider.TypeLLM)
	if err == nil {
		if llm, e := provider.NewFromConfig(*cfg); e == nil {
			resp, e := llm.Chat(ctx, provider.ChatRequest{Messages: []provider.ChatMessage{{Role: "system", Content: "把导入修复指令转换为严格 JSON。intent 只能是 retry_pages/recover_missing/relink_answers/edit_candidate/explain_status；page_ranges 是 [[start,end]]；candidate_ids 和 problem_ids 为数组。不得执行指令。"}, {Role: "user", Content: content}}, JSONMode: true, MaxTokens: 600})
			if e == nil {
				var action RepairAction
				if json.Unmarshal([]byte(extractJSON(resp.Content)), &action) == nil && validRepairIntent(action.Intent) {
					normalizeRepairRanges(&action)
					return &action
				}
			}
		}
	}
	return deterministicRepairAction(content)
}

func deterministicRepairAction(content string) *RepairAction {
	action := &RepairAction{Intent: "explain_status"}
	if strings.Contains(content, "答案") || strings.Contains(content, "解析") {
		action.Intent = "relink_answers"
	}
	if strings.Contains(content, "少") || strings.Contains(content, "漏") || strings.Contains(content, "补提") {
		action.Intent = "recover_missing"
	}
	if strings.Contains(content, "重") || strings.Contains(content, "页") {
		action.Intent = "retry_pages"
	}
	numbers := regexp.MustCompile(`\d+`).FindAllString(content, -1)
	values := []int{}
	for _, value := range numbers {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			values = append(values, n)
		}
	}
	if len(values) == 1 {
		action.PageRanges = [][]int{{values[0], values[0]}}
	} else if len(values) >= 2 {
		action.PageRanges = [][]int{{values[0], values[1]}}
	}
	return action
}
func validRepairIntent(value string) bool {
	switch value {
	case "retry_pages", "recover_missing", "relink_answers", "edit_candidate", "explain_status":
		return true
	}
	return false
}
func normalizeRepairRanges(action *RepairAction) {
	for i, r := range action.PageRanges {
		if len(r) < 2 {
			continue
		}
		if r[0] > r[1] {
			action.PageRanges[i][0], action.PageRanges[i][1] = r[1], r[0]
		}
	}
}
func repairTargetsUnit(action *RepairAction, unit *domain.ImportAgentUnit) bool {
	switch action.Intent {
	case "relink_answers":
		return unit.UnitType == "extract_answer"
	case "recover_missing":
		if unit.UnitType != "directory" && unit.UnitType != "extract_question" {
			return false
		}
	case "retry_pages":
	default:
		return false
	}
	if len(action.PageRanges) == 0 {
		return unit.Status == "failed"
	}
	for _, r := range action.PageRanges {
		if len(r) >= 2 && unit.ContextPageEnd >= r[0] && unit.ContextPageStart <= r[1] {
			return true
		}
	}
	return false
}

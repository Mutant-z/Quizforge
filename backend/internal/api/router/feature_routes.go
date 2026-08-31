package router

import (
	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/api/handler"
	"github.com/quiztrace/quiztrace/internal/api/middleware"
	"github.com/quiztrace/quiztrace/internal/config"
	"github.com/quiztrace/quiztrace/internal/repository/sqlite"
	"github.com/quiztrace/quiztrace/internal/service"
)

func registerUploadRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	h := handler.NewUploadHandler(repo, cfg)

	u := g.Group("/uploads", middleware.Auth(deps.Token))
	u.POST("/init", h.Init)
	u.POST("/:id/chunks/:index", h.UploadChunk)
	u.POST("/:id/complete", h.Complete)
}

func registerImportRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	h := handler.NewImportHandler(repo, cfg)

	im := g.Group("/imports", middleware.Auth(deps.Token))
	im.GET("", h.List)
	im.GET("/:id", h.Get)
	im.GET("/:id/events", h.Events)
	im.GET("/:id/draft", h.Draft)
	im.PATCH("/:id/candidates/:candidate_id", h.UpdateCandidate)
	im.GET("/:id/messages", h.Messages)
	im.POST("/:id/messages", h.CreateMessage)
	im.POST("/:id/retry", h.Retry)
	im.POST("/:id/reprocess-vision", h.ReprocessVision)
	im.POST("/:id/preview", h.Preview)
	im.POST("/:id/commit", h.Commit)
	im.GET("/:id/pages/:page", h.PageImage)
}

func registerImportAgentRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	h := handler.NewImportAgentHandler(repo, cfg)
	upload := handler.NewUploadHandler(repo, cfg)

	a := g.Group("/import-agent", middleware.Auth(deps.Token))
	a.POST("/sessions", h.Create)
	a.GET("/sessions", h.List)
	a.GET("/sessions/:id", h.Get)
	a.PATCH("/sessions/:id", h.Patch)
	a.DELETE("/sessions/:id", h.Archive)
	a.POST("/sessions/:id/uploads/init", upload.Init)
	a.POST("/sessions/:id/uploads/:upload_id/chunks/:index", upload.UploadChunk)
	a.POST("/sessions/:id/uploads/:upload_id/complete", upload.Complete)
	a.GET("/sessions/:id/messages", h.Messages)
	a.POST("/sessions/:id/messages/stream", h.MessageStream)
	a.GET("/sessions/:id/events", h.Events)
	a.GET("/sessions/:id/draft", h.Draft)
	a.PATCH("/sessions/:id/candidates/:candidate_id", h.UpdateCandidate)
	a.PATCH("/sessions/:id/candidates/:candidate_id/review", h.ReviewCandidate)
	a.POST("/sessions/:id/candidates/review/batch", h.ReviewCandidatesBatch)
	a.POST("/sessions/:id/candidates/reextract", h.ReextractCandidates)
	a.GET("/sessions/:id/candidates/:candidate_id/provenance", h.CandidateProvenance)
	a.POST("/sessions/:id/relink-answers", h.RelinkAnswers)
	a.POST("/sessions/:id/retry-answer-pages", h.RetryAnswerPages)
	a.POST("/sessions/:id/actions/:action_id/confirm", h.ConfirmAction)
	a.POST("/sessions/:id/preview", h.Preview)
	a.POST("/sessions/:id/commit", h.Commit)
	a.POST("/sessions/:id/runs/:run_id/cancel", h.CancelRun)
	a.GET("/sessions/:id/runs/:run_id/audit", h.Audit)
	a.GET("/sessions/:id/documents/:document_id/pages/:page", h.PageImage)
	a.PATCH("/sessions/:id/documents/:document_id/role", h.UpdateDocumentRole)
}

func registerWrongImportRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	h := handler.NewWrongImportHandler(repo, cfg)

	w := g.Group("/wrong-import", middleware.Auth(deps.Token))
	w.POST("/sessions", h.Create)
	w.GET("/sessions", h.List)
	w.GET("/sessions/:id", h.Get)
	w.DELETE("/sessions/:id", h.Discard)
	w.POST("/sessions/:id/batches/:batch_id/finalize", h.FinalizeBatch)
	w.POST("/sessions/:id/images/init", h.InitImage)
	w.POST("/sessions/:id/images/:image_id/chunks/:index", h.UploadImageChunk)
	w.POST("/sessions/:id/images/:image_id/complete", h.CompleteImage)
	w.POST("/sessions/:id/images/:image_id/extract", h.ExtractImage)
	w.GET("/sessions/:id/images/:image_id/file", h.ImageFile)
	w.GET("/sessions/:id/events", h.Events)
	w.GET("/sessions/:id/drafts", h.ListDrafts)
	w.POST("/sessions/:id/reconcile", h.Reconcile)
	w.POST("/sessions/:id/answer-reconcile", h.RetryAnswerExtraction)
	w.GET("/sessions/:id/merge-candidates", h.ListMergeCandidates)
	w.POST("/sessions/:id/merge-candidates/:candidate_id/resolve", h.ResolveMergeCandidate)
	w.POST("/sessions/:id/merges/:merge_id/undo", h.UndoMerge)
	w.PATCH("/sessions/:id/drafts/:draft_id", h.PatchDraft)
	w.DELETE("/sessions/:id/drafts/:draft_id", h.DeleteDraft)
	w.GET("/sessions/:id/messages", h.ListMessages)
	w.POST("/sessions/:id/messages", h.SendMessage)
	w.POST("/sessions/:id/confirm", h.Confirm)
}

func registerRAGRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	svc := service.NewRAGService(repo, cfg)
	h := handler.NewRAGHandler(svc)

	rag := g.Group("/rag", middleware.Auth(deps.Token))
	rag.POST("/search", h.Search)

	search := g.Group("/search", middleware.Auth(deps.Token))
	search.GET("/questions", func(c *gin.Context) {
		q := c.Query("q")
		request := service.SearchRequest{Query: q, TopK: 10}
		if role, _ := c.Get("role"); role != "admin" {
			request.UserID = middleware.CurrentUserID(c)
		}
		res, err := svc.Search(c.Request.Context(), request)
		if err != nil {
			handler.RespondJSON(c, gin.H{"items": []interface{}{}})
			return
		}
		handler.RespondJSON(c, res)
	})
}

func registerAgentRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	svc := service.NewAgentService(repo, cfg)
	h := handler.NewAgentHandler(svc, repo)

	a := g.Group("/agent", middleware.Auth(deps.Token))
	a.POST("/sessions", h.CreateSession)
	a.GET("/sessions", h.ListSessions)
	a.GET("/sessions/:id", h.GetSession)
	a.DELETE("/sessions/:id", h.DeleteSession)
	a.POST("/sessions/:id/messages", h.Stream)
	// Keep the old mobile client path working while clients update to /messages.
	a.POST("/sessions/:id/chat", h.Stream)
	a.GET("/sessions/:id/stream", h.Stream)
}

func registerProviderRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	registry := newProviderRegistry(deps, cfg)
	h := handler.NewProviderHandler(registry)

	p := g.Group("/providers", middleware.Auth(deps.Token))
	p.GET("", h.List)
	p.POST("", h.Save)
	p.PUT("/:id", h.Update)
	p.POST("/:id/default", h.SetDefault)
	p.DELETE("/:id", h.Delete)
	p.POST("/test", h.Test)
	p.POST("/:id/test", h.TestStored)
}

func registerAdminRoutes(g *gin.RouterGroup, deps *Deps, cfg *config.Config) {
	repo := sqlite.NewRepository(deps.DB)
	svc := service.NewQuestionService(repo)
	admin := handler.NewAdminHandler(repo, cfg)
	qh := handler.NewAdminQuestionHandler(svc, repo)

	ad := g.Group("/admin", middleware.Auth(deps.Token), middleware.RequireRole("admin"))
	ad.GET("/candidates", admin.ListCandidates)
	ad.GET("/candidates/:id", admin.GetCandidate)
	ad.POST("/candidates/:id/approve", admin.ApproveCandidate)
	ad.POST("/candidates/:id/reject", admin.RejectCandidate)
	ad.GET("/conflicts", admin.ListConflicts)
	ad.POST("/conflicts/:id/resolve", admin.ResolveConflict)
	ad.GET("/jobs", admin.ListJobs)
	ad.GET("/questions", qh.List)
	ad.POST("/questions", qh.Create)
	ad.PUT("/questions/:id", qh.Update)
	ad.DELETE("/questions/:id", qh.Delete)
	ad.POST("/questions/:id/reindex", admin.ReindexQuestion)
}

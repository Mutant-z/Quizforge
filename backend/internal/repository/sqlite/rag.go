package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/quiztrace/quiztrace/internal/domain"
)

// RAGDocument 检索结果。
type RAGDocument struct {
	ID           int64   `json:"id"`
	SourceType   string  `json:"source_type"`
	QuestionID   int64   `json:"question_id,omitempty"`
	DocumentID   int64   `json:"document_id,omitempty"`
	DocumentPage int     `json:"document_page,omitempty"`
	BankID       int64   `json:"bank_id,omitempty"`
	SubjectID    int64   `json:"subject_id,omitempty"`
	ChapterID    int64   `json:"chapter_id,omitempty"`
	KnowledgePoint string `json:"knowledge_point,omitempty"`
	QuestionType string  `json:"question_type,omitempty"`
	Difficulty   float64 `json:"difficulty,omitempty"`
	Content      string  `json:"content"`
	Score        float64 `json:"score"`
}

// UpsertRAGDocument 写入 RAG 文档（内容 hash 去重，幂等）。
func (r *Repository) UpsertRAGDocument(ctx context.Context, sourceType string, questionID int64, documentID *int64, page int, bankID int64, subjectID, chapterID *int64, kp string, difficulty float64, qType, content string) error {
	hash := contentHash(content)
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM rag_documents WHERE source_type = ? AND content_hash = ?`, sourceType, hash).Scan(&id)
	if err == nil {
		// 更新 stale 标记与元数据
		_, err = r.db.ExecContext(ctx, `
			UPDATE rag_documents SET question_id = ?, document_id = ?, document_page = ?, bank_id = ?, subject_id = ?,
				chapter_id = ?, knowledge_point = ?, difficulty = ?, question_type = ?, content = ?, stale = 0,
				updated_at = datetime('now')
			WHERE id = ?`, questionID, documentID, page, bankID, subjectID, chapterID, kp, difficulty, qType, content, id)
		return err
	}
	if err != sql.ErrNoRows {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO rag_documents (source_type, question_id, document_id, document_page, bank_id, subject_id, chapter_id, knowledge_point, difficulty, question_type, content, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sourceType, questionID, documentID, page, bankID, subjectID, chapterID, kp, difficulty, qType, content, hash)
	if err != nil {
		return fmt.Errorf("insert rag doc: %w", err)
	}
	// FTS 同步索引
	if err := r.reindexFTS(ctx, hash); err != nil {
		return err
	}
	return nil
}

// UpsertEmbedding 写入 / 更新向量（按 rag_document_id + model）。
func (r *Repository) UpsertEmbedding(ctx context.Context, ragDocID int64, model string, vector []float32) error {
	blob := float32sToBytes(vector)
	// 先更新已存在记录
	res, err := r.db.ExecContext(ctx, `
		UPDATE rag_embeddings SET model = ?, dimension = ?, vector = ?, created_at = datetime('now')
		WHERE rag_document_id = ? AND model = ?`, model, len(vector), blob, ragDocID, model)
	if err != nil {
		return fmt.Errorf("upsert embedding: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO rag_embeddings (rag_document_id, model, dimension, vector)
		VALUES (?, ?, ?, ?)`, ragDocID, model, len(vector), blob)
	if err != nil {
		return fmt.Errorf("upsert embedding insert: %w", err)
	}
	return nil
}

func (r *Repository) reindexFTS(ctx context.Context, contentHash string) error {
	var id int64
	var sourceType, content string
	var bankID, subjectID, chapterID, qID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, source_type, content, bank_id, subject_id, chapter_id, question_id
		FROM rag_documents WHERE content_hash = ?`, contentHash).Scan(&id, &sourceType, &content, &bankID, &subjectID, &chapterID, &qID)
	if err != nil {
		return err
	}
	// FTS5 列存字符串，元数据统一转字符串便于精确过滤
	_, err = r.db.ExecContext(ctx, `INSERT OR REPLACE INTO rag_fts (rowid, content, source_type, bank_id, subject_id, chapter_id, question_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, content, sourceType, strOrEmpty(bankID), strOrEmpty(subjectID), strOrEmpty(chapterID), strOrEmpty(qID))
	if err != nil {
		return fmt.Errorf("reindex fts: %w", err)
	}
	return nil
}

func strOrEmpty(n sql.NullInt64) string {
	if n.Valid {
		return fmt.Sprintf("%d", n.Int64)
	}
	return ""
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:16])
}

// SearchFTS 关键词检索（FTS5 + BM25）。
func (r *Repository) SearchFTS(ctx context.Context, query string, topK int, bankID, subjectID, chapterID *int64) ([]RAGDocument, error) {
	if topK <= 0 {
		topK = 20
	}
	where := []string{"rag_fts MATCH ?"}
	args := []interface{}{query}
	if bankID != nil {
		where = append(where, "rag_fts.bank_id = ?")
		args = append(args, fmt.Sprintf("%d", *bankID))
	}
	if subjectID != nil {
		where = append(where, "rag_fts.subject_id = ?")
		args = append(args, fmt.Sprintf("%d", *subjectID))
	}
	if chapterID != nil {
		where = append(where, "rag_fts.chapter_id = ?")
		args = append(args, *chapterID)
	}
	whereSQL := strings.Join(where, " AND ")

	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.source_type, d.question_id, d.document_id, d.document_page, d.bank_id, d.subject_id,
		       d.chapter_id, d.knowledge_point, d.question_type, d.difficulty, d.content, bm25(rag_fts) * -1 AS score
		FROM rag_fts JOIN rag_documents d ON d.id = rag_fts.rowid
		WHERE `+whereSQL+`
		ORDER BY score DESC LIMIT ?`, append(args, topK)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRAGDocuments(rows)
}

// GetRAGDocumentByQuestion 按题目获取 RAG 文档（当前题优先）。
func (r *Repository) GetRAGDocumentByQuestion(ctx context.Context, questionID int64) (*RAGDocument, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, source_type, question_id, document_id, document_page, bank_id, subject_id, chapter_id,
		       knowledge_point, question_type, difficulty, content, 1.0
		FROM rag_documents WHERE question_id = ? AND source_type = 'question' LIMIT 1`, questionID)
	var d RAGDocument
	var docID, page sql.NullInt64
	var kp, qType sql.NullString
	var subjectID, chapterID sql.NullInt64
	var difficulty sql.NullFloat64
	err := row.Scan(&d.ID, &d.SourceType, &d.QuestionID, &docID, &page, &d.BankID, &subjectID, &chapterID, &kp, &qType, &difficulty, &d.Content, &d.Score)
	if err != nil {
		return nil, err
	}
	d.DocumentID = docID.Int64
	d.DocumentPage = int(page.Int64)
	d.SubjectID = subjectID.Int64
	d.ChapterID = chapterID.Int64
	d.KnowledgePoint = kp.String
	d.QuestionType = qType.String
	d.Difficulty = difficulty.Float64
	return &d, nil
}

// SearchVector 向量相似度检索（线性扫描，万级数据量可接受）。
func (r *Repository) SearchVector(ctx context.Context, query []float32, topK int, bankID, subjectID, chapterID *int64) ([]RAGDocument, error) {
	if topK <= 0 {
		topK = 20
	}
	where := []string{"e.vector IS NOT NULL", "d.stale = 0"}
	args := []interface{}{}
	if bankID != nil {
		where = append(where, "d.bank_id = ?")
		args = append(args, *bankID)
	}
	if subjectID != nil {
		where = append(where, "d.subject_id = ?")
		args = append(args, *subjectID)
	}
	if chapterID != nil {
		where = append(where, "d.chapter_id = ?")
		args = append(args, *chapterID)
	}
	whereSQL := strings.Join(where, " AND ")

	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.source_type, d.question_id, d.document_id, d.document_page, d.bank_id, d.subject_id,
		       d.chapter_id, d.knowledge_point, d.question_type, d.difficulty, d.content, e.vector
		FROM rag_embeddings e JOIN rag_documents d ON d.id = e.rag_document_id
		WHERE `+whereSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type cand struct {
		doc  RAGDocument
		sim  float64
	}
	results := []cand{}
	for rows.Next() {
		var d RAGDocument
		var docID, page sql.NullInt64
		var kp, qType sql.NullString
		var subjectID, chapterID sql.NullInt64
		var difficulty sql.NullFloat64
		var vecBlob []byte
		if err := rows.Scan(&d.ID, &d.SourceType, &d.QuestionID, &docID, &page, &d.BankID, &subjectID, &chapterID, &kp, &qType, &difficulty, &d.Content, &vecBlob); err != nil {
			return nil, err
		}
		d.DocumentID = docID.Int64
		d.DocumentPage = int(page.Int64)
		d.SubjectID = subjectID.Int64
		d.ChapterID = chapterID.Int64
		d.KnowledgePoint = kp.String
		d.QuestionType = qType.String
		d.Difficulty = difficulty.Float64
		vec := bytesToFloat32s(vecBlob)
		if len(vec) != len(query) {
			continue
		}
		sim := cosineSimilarity(query, vec)
		results = append(results, cand{doc: d, sim: sim})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 排序取 topK
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].sim > results[j-1].sim; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	out := make([]RAGDocument, 0, min(topK, len(results)))
	for i := 0; i < len(results) && i < topK; i++ {
		results[i].doc.Score = results[i].sim
		out = append(out, results[i].doc)
	}
	return out, nil
}

func cosineSimilarity(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// GetEmbeddingDimension 获取当前 embedding 维度（无则返回 0）。
func (r *Repository) GetEmbeddingDimension(ctx context.Context, model string) (int, error) {
	var dim int
	err := r.db.QueryRowContext(ctx, `SELECT dimension FROM rag_embeddings WHERE model = ? LIMIT 1`, model).Scan(&dim)
	if err != nil {
		return 0, err
	}
	return dim, nil
}

// CountRAGDocuments 统计。
func (r *Repository) CountRAGDocuments(ctx context.Context) (int64, error) {
	var c int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rag_documents WHERE stale = 0`).Scan(&c)
	return c, err
}

// ListRAGDocumentsForEmbedding 需要 embedding 的文档（按 id 升序分批）。
func (r *Repository) ListRAGDocumentsForEmbedding(ctx context.Context, model string, limit, offset int) ([]RAGDocument, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.source_type, d.question_id, d.document_id, d.document_page, d.bank_id, d.subject_id,
		       d.chapter_id, d.knowledge_point, d.question_type, d.difficulty, d.content, 0
		FROM rag_documents d
		WHERE d.stale = 0 AND d.id NOT IN (SELECT rag_document_id FROM rag_embeddings WHERE model = ?)
		ORDER BY d.id LIMIT ? OFFSET ?`, model, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRAGDocuments(rows)
}

func scanRAGDocuments(rows *sql.Rows) ([]RAGDocument, error) {
	docs := []RAGDocument{}
	for rows.Next() {
		var d RAGDocument
		var docID, page sql.NullInt64
		var kp, qType sql.NullString
		var subjectID, chapterID sql.NullInt64
		var difficulty sql.NullFloat64
		if err := rows.Scan(&d.ID, &d.SourceType, &d.QuestionID, &docID, &page, &d.BankID, &subjectID, &chapterID, &kp, &qType, &difficulty, &d.Content, &d.Score); err != nil {
			return nil, err
		}
		d.DocumentID = docID.Int64
		d.DocumentPage = int(page.Int64)
		d.SubjectID = subjectID.Int64
		d.ChapterID = chapterID.Int64
		d.KnowledgePoint = kp.String
		d.QuestionType = qType.String
		d.Difficulty = difficulty.Float64
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// float32sToBytes / bytesToFloat32s BLOB 序列化。
func float32sToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		u := math.Float32bits(f)
		buf[i*4] = byte(u)
		buf[i*4+1] = byte(u >> 8)
		buf[i*4+2] = byte(u >> 16)
		buf[i*4+3] = byte(u >> 24)
	}
	return buf
}

func bytesToFloat32s(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		u := uint32(b[i*4]) | uint32(b[i*4+1])<<8 | uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		out[i] = math.Float32frombits(u)
	}
	return out
}

var _ = json.Marshal
var _ = domain.QuestionStatusPublished

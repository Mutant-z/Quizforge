# Example Structured Output Schema

## Extractor Result

```json
{
  "schema_version": "question-extractor.v1",
  "candidate_id": 1001,
  "type": "single_choice",
  "stem": "以下说法正确的是？",
  "options": [
    {"key": "A", "content": "..."},
    {"key": "B", "content": "..."}
  ],
  "answer": ["B"],
  "analysis": "原文解析",
  "source_pages": [31, 32],
  "confidence": 0.96,
  "warnings": []
}
```

## Validator Result

```json
{
  "valid": true,
  "confidence": 0.97,
  "issues": [],
  "suggested_action": "accept"
}
```

## Classification Result

```json
{
  "subject_id": 1,
  "chapter_id": 12,
  "knowledge_points": [
    "ConcurrentHashMap",
    "CAS"
  ],
  "difficulty": 3,
  "suggest_new_chapter": false
}
```

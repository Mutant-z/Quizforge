-- Speed up draft pagination when resolving the physical reading-order anchor
-- for each candidate.  The directory is already scoped by session/document;
-- keep the page/question identity columns in the index so the draft endpoint
-- does not execute a full directory scan for every card.
CREATE INDEX IF NOT EXISTS idx_directory_physical_order
    ON import_directory_entries(
        session_id,
        document_id,
        content_role,
        status,
        source_page_start,
        source_page_end,
        source_question_no,
        canonical_question_type,
        marker_top
    );

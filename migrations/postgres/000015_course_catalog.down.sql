-- Catalog and schedule data are retained on rollback. Reverting the application
-- must not discard the only Postgres copy of metadata imported from Notion.
SELECT 1;

-- Intentionally retain email_opt_outs when rolling back application code.
-- Dropping it would lose unsubscribe preferences needed by future imports.
SELECT 1;

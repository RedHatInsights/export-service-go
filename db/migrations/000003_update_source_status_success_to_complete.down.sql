-- Revert source status: 'complete' → 'success'
UPDATE sources SET status = 'success' WHERE status = 'complete';

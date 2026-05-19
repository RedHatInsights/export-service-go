-- Fix source status to match OpenAPI spec: 'success' → 'complete'
UPDATE sources SET status = 'complete' WHERE status = 'success';

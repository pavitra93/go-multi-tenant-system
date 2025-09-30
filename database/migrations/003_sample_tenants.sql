-- =====================================================
-- SAMPLE TENANT DATA
-- For development and testing
-- =====================================================

-- Insert sample tenant
INSERT INTO tenants (id, name, created_at, updated_at)
VALUES (
    '550e8400-e29b-41d4-a716-446655440001',
    'Acme Corporation',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (id) DO NOTHING;

-- =====================================================
-- NOTES
-- =====================================================
-- 
-- This tenant will be used for:
-- - Development testing
-- - API testing
-- - Load testing
-- 
-- Users are created through Cognito registration, not SQL
-- Location sessions and locations are created through API calls
-- 
-- =====================================================


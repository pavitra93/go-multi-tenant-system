-- =====================================================
-- SAMPLE DATA FOR DEVELOPMENT & TESTING
-- =====================================================

-- =====================================================
-- SAMPLE TENANTS
-- =====================================================
INSERT INTO tenants (id, name, domain, is_active, created_at, updated_at) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', 'Acme Corporation', 'acme.com', true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('fa94a1a4-b241-4777-89d4-703948210b20', 'TechStart Inc', 'techstart.io', true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('5d691765-0c59-4593-96f7-b591a7bcaae7', 'Apple Inc', 'apple.io', true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (id) DO NOTHING;

-- =====================================================
-- NOTES FOR INTERVIEWER
-- =====================================================
-- 
-- 1. USER MANAGEMENT:
--    - Admin users are created in the 'admins' table
--    - Tenant users are created in the 'users' table with tenant_id
--    - User authentication is handled by AWS Cognito
--    - User data (email, username) is stored in Cognito, not in our database
--
-- 2. LOCATION TRACKING:
--    - Users start a 10-minute location session
--    - Location points are submitted every 30 seconds during the session
--    - Each location point is stored with latitude, longitude, and accuracy
--    - Sessions can be active, completed, or expired
--
-- 3. MULTI-TENANCY:
--    - Row Level Security (RLS) ensures tenant data isolation
--    - Each tenant can only access their own data
--    - Admin users can access all tenant data
--
-- 4. PERFORMANCE:
--    - Strategic indexes optimize the most common queries
--    - Session validation is the most critical performance path
--    - Location inserts happen frequently (every 30 seconds per active user)
--
-- =====================================================

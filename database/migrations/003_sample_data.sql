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
-- SAMPLE DATA COMPLETE
-- =====================================================

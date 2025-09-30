-- Insert sample tenant data
INSERT INTO tenants (id, name, domain) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', 'Acme Corporation', 'acme.com'),
    ('550e8400-e29b-41d4-a716-446655440002', 'TechStart Inc', 'techstart.io'),
    ('550e8400-e29b-41d4-a716-446655440003', 'Global Logistics', 'globallogistics.com')
ON CONFLICT (id) DO NOTHING;

-- Insert sample admin user
INSERT INTO users (id, tenant_id, username, role, cognito_id) VALUES
    ('650e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'admin', 'admin', 'cognito-admin-001')
ON CONFLICT (id) DO NOTHING;

-- Insert sample regular users
INSERT INTO users (id, tenant_id, username, role, cognito_id) VALUES
    ('650e8400-e29b-41d4-a716-446655440002', '550e8400-e29b-41d4-a716-446655440001', 'johndoe', 'user', 'cognito-user-001'),
    ('650e8400-e29b-41d4-a716-446655440003', '550e8400-e29b-41d4-a716-446655440001', 'janesmith', 'user', 'cognito-user-002'),
    ('650e8400-e29b-41d4-a716-446655440004', '550e8400-e29b-41d4-a716-446655440002', 'bobwilson', 'user', 'cognito-user-003'),
    ('650e8400-e29b-41d4-a716-446655440005', '550e8400-e29b-41d4-a716-446655440003', 'alicebrown', 'user', 'cognito-user-004')
ON CONFLICT (id) DO NOTHING;

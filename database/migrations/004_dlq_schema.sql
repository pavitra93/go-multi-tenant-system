-- =====================================================
-- DLQ SCHEMA
-- Dead Letter Queue for failed location updates
-- =====================================================

-- Failed messages table for DLQ
CREATE TABLE IF NOT EXISTS failed_location_updates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_event_id VARCHAR(255) NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    session_id UUID,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    error_message TEXT NOT NULL,
    retry_count INTEGER DEFAULT 0,
    status VARCHAR(50) DEFAULT 'pending', -- pending, retried, resolved, permanently_failed
    next_retry_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for failed messages
CREATE INDEX IF NOT EXISTS idx_failed_location_updates_tenant_id ON failed_location_updates(tenant_id);
CREATE INDEX IF NOT EXISTS idx_failed_location_updates_status ON failed_location_updates(status);
CREATE INDEX IF NOT EXISTS idx_failed_location_updates_created_at ON failed_location_updates(created_at);
CREATE INDEX IF NOT EXISTS idx_failed_location_updates_original_event_id ON failed_location_updates(original_event_id);
CREATE INDEX IF NOT EXISTS idx_failed_location_updates_next_retry ON failed_location_updates(next_retry_at);
CREATE INDEX IF NOT EXISTS idx_failed_location_updates_retry_lookup ON failed_location_updates(status, next_retry_at, created_at DESC);

-- Enable RLS on failed messages table
ALTER TABLE failed_location_updates ENABLE ROW LEVEL SECURITY;

-- RLS Policy for failed messages: Only allow access to failed messages within the current tenant
CREATE POLICY failed_messages_isolation_policy ON failed_location_updates
    USING (tenant_id = current_setting('app.current_tenant_id', TRUE)::UUID);

-- =====================================================
-- DLQ SCHEMA COMPLETE
-- =====================================================

-- Gotenberg Manager: initial schema

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    api_key VARCHAR(255) UNIQUE NOT NULL,
    tyk_key_id VARCHAR(255) DEFAULT '',
    plan VARCHAR(50) NOT NULL DEFAULT 'free',
    monthly_limit INT NOT NULL DEFAULT 100,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS usage_records (
    id BIGSERIAL PRIMARY KEY,
    client_id UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    endpoint VARCHAR(512) NOT NULL,
    status_code INT NOT NULL DEFAULT 0,
    response_time_ms INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS health_checks (
    id BIGSERIAL PRIMARY KEY,
    service VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL,
    response_time_ms INT NOT NULL DEFAULT 0,
    details TEXT DEFAULT '',
    checked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_usage_client_id ON usage_records(client_id);
CREATE INDEX IF NOT EXISTS idx_usage_created_at ON usage_records(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_client_date ON usage_records(client_id, created_at);
CREATE INDEX IF NOT EXISTS idx_health_service ON health_checks(service);
CREATE INDEX IF NOT EXISTS idx_health_checked_at ON health_checks(checked_at);
CREATE INDEX IF NOT EXISTS idx_clients_api_key ON clients(api_key);
CREATE INDEX IF NOT EXISTS idx_clients_active ON clients(is_active);

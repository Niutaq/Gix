-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- 'cantors' and 'rates' tables
CREATE TABLE IF NOT EXISTS cantors (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    base_url TEXT NOT NULL,
    strategy VARCHAR(10) NOT NULL,
    units INTEGER DEFAULT 1,
    latitude DECIMAL(9,6) DEFAULT 0,
    longitude DECIMAL(9,6) DEFAULT 0,
    address TEXT
);

CREATE TABLE IF NOT EXISTS rates (
    time TIMESTAMPTZ NOT NULL,
    cantor_id INTEGER NOT NULL REFERENCES cantors(id),
    currency VARCHAR(3) NOT NULL,
    buy_rate NUMERIC(10, 4) NOT NULL,
    sell_rate NUMERIC(10, 4) NOT NULL,
    UNIQUE (time, cantor_id, currency)
);

-- FinOps: Table for Unit Economics Tracking (FOCUS 1.0 Aligned)
CREATE TABLE IF NOT EXISTS provider_unit_costs (
    time TIMESTAMPTZ NOT NULL,
    provider_id VARCHAR(50) NOT NULL,
    scraper_type VARCHAR(20) NOT NULL,
    duration_ms BIGINT NOT NULL,
    estimated_cost_usd NUMERIC(15, 10) NOT NULL,
    trace_id VARCHAR(100),
    service_category VARCHAR(50),
    resource_name VARCHAR(100)
);

-- FinOps: FOCUS-shaped showback / allocation rows (OpenCost sync + app mirror)
CREATE TABLE IF NOT EXISTS focus_cost_records (
    time TIMESTAMPTZ NOT NULL,
    provider_name VARCHAR(50) NOT NULL DEFAULT 'OpenCost',
    source VARCHAR(50) NOT NULL,
    billed_cost NUMERIC(15, 10),
    effective_cost NUMERIC(15, 10) NOT NULL,
    service_category VARCHAR(50),
    resource_name VARCHAR(100),
    x_application VARCHAR(50) DEFAULT 'gix',
    x_team VARCHAR(50) DEFAULT 'platform',
    x_environment VARCHAR(50) DEFAULT 'local',
    charge_description TEXT
);

-- Convert to hypertables
SELECT create_hypertable('rates', 'time', if_not_exists => TRUE);
SELECT create_hypertable('provider_unit_costs', 'time', if_not_exists => TRUE);
SELECT create_hypertable('focus_cost_records', 'time', if_not_exists => TRUE);

-- FinOps: Data Retention Policies to control storage costs
SELECT add_retention_policy('rates', INTERVAL '30 days', if_not_exists => TRUE);
SELECT add_retention_policy('provider_unit_costs', INTERVAL '60 days', if_not_exists => TRUE);
SELECT add_retention_policy('focus_cost_records', INTERVAL '90 days', if_not_exists => TRUE);

-- Clean up data (optional, for development)
TRUNCATE TABLE rates, cantors RESTART IDENTITY CASCADE;

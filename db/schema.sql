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
    longitude DECIMAL(9,6) DEFAULT 0
);

CREATE TABLE IF NOT EXISTS rates (
    time TIMESTAMPTZ NOT NULL,
    cantor_id INTEGER NOT NULL REFERENCES cantors(id),
    currency VARCHAR(3) NOT NULL,
    buy_rate NUMERIC(10, 4) NOT NULL,
    sell_rate NUMERIC(10, 4) NOT NULL,
    UNIQUE (time, cantor_id, currency)
);

-- Convert 'rates' to hypertable
SELECT create_hypertable('rates', 'time', if_not_exists => TRUE);

-- Clean up data (optional, for development)
TRUNCATE TABLE rates, cantors RESTART IDENTITY CASCADE;

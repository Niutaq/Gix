-- db/schema.sql

-- TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- 'cantors' and 'rates' tables
CREATE TABLE IF NOT EXISTS cantors (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    base_url TEXT NOT NULL,
    strategy VARCHAR(10) NOT NULL,
    units INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rates (
    time TIMESTAMPTZ NOT NULL,
    cantor_id INTEGER NOT NULL REFERENCES cantors(id),
    currency VARCHAR(3) NOT NULL,
    buy_rate NUMERIC(10, 4) NOT NULL,
    sell_rate NUMERIC(10, 4) NOT NULL,

    UNIQUE (time, cantor_id, currency)
);

TRUNCATE TABLE rates, cantors RESTART IDENTITY CASCADE;

-- Hypertable
SELECT create_hypertable('rates', 'time', if_not_exists => TRUE);
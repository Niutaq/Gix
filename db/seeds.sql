-- db/seeds.sql

INSERT INTO cantors (name, display_name, base_url, strategy, units) VALUES
('tadek', 'Tadek', 'https://kantorstalowawola.tadek.pl/', 'C1', 100),
('kantorexchange', 'Exchange', 'https://kantorywalut-rzeszow.pl/kursy-walut', 'C2', 1),
('supersam', 'Supersam', 'http://www.kantorsupersam.pl/', 'C3', 1)
ON CONFLICT (name) DO NOTHING;

SELECT * FROM cantors;

SELECT * FROM rates;
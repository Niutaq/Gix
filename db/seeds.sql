-- Seeds for cantors
INSERT INTO cantors (name, display_name, base_url, strategy, units, latitude, longitude) VALUES
('tadek', 'Kantor Tadek', 'https://kantorstalowawola.tadek.pl/', 'C1', 100, 50.5826, 22.0537),
('exchange', 'Kantor Exchange', 'https://kantorywalut-rzeszow.pl/kursy-walut', 'C2', 1, 50.0413, 21.9990),
('supersam', 'Kantor Supersam', 'http://www.kantorsupersam.pl/', 'C3', 1, 50.0375, 22.0040)
ON CONFLICT (name) DO NOTHING;
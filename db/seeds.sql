-- Seeds for cantors
INSERT INTO cantors (name, display_name, base_url, strategy, units, latitude, longitude) VALUES
('tadek', 'Kantor Tadek (Stalowa Wola)', 'https://kantorstalowawola.tadek.pl/', 'C1', 100, 50.5826, 22.0537),
('exchange', 'Kantor Kwadrat (Rzeszów)', 'https://kantorywalut-rzeszow.pl/kursy-walut', 'C2', 1, 50.0413, 21.9990),
('supersam', 'Kantor Supersam (Rzeszów)', 'http://www.kantorsupersam.pl/', 'C3', 1, 50.0375, 22.0040),
-- ('alex', 'Kantor Alex (Rzeszów)', 'https://kantoralex.rzeszow.pl/', 'C4', 1, 50.0267, 22.0163),
('grosz', 'Kantor Grosz (Kraków)', 'https://kantorgrosz.pl/', 'C5', 100, 50.0632, 19.9392),
('centrum', 'Kantor Centrum (Wrocław)', 'https://kantor-centrum.pl/', 'C6', 1, 51.1085, 17.0331)
ON CONFLICT (name) DO NOTHING;
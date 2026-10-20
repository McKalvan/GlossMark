-- @gloss represents dine:Location conf=0.91
CREATE TABLE locations (
    id          uuid PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    timezone    text NOT NULL DEFAULT 'Europe/London',
    opened_on   date NOT NULL
);

INSERT INTO locations (id, name, opened_on) VALUES
    ('6f1d2c3e-0000-4000-8000-000000000001', 'Pepper Row Canal Street', '2019-04-12'),
    ('6f1d2c3e-0000-4000-8000-000000000002', 'Pepper Row Harbour', '2022-06-03'),
    ('6f1d2c3e-0000-4000-8000-000000000003', 'Pepper Row Market Hall', '2025-02-20');

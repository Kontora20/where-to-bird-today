-- +goose Up

-- Lets the species picker match "zuvedra" to "Žuvėdra": Lithuanian names
-- carry diacritics that people often type without.
CREATE EXTENSION IF NOT EXISTS unaccent;

-- +goose Down

DROP EXTENSION unaccent;

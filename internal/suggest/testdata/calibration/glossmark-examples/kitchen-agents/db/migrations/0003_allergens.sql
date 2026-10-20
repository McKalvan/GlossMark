CREATE TYPE allergen AS ENUM (
    'celery', 'gluten', 'crustaceans', 'eggs', 'fish', 'lupin', 'milk',
    'molluscs', 'mustard', 'tree_nuts', 'peanuts', 'sesame', 'soya', 'sulphites'
);

CREATE TYPE allergen_kind AS ENUM ('contains', 'may_contain');

-- @gloss represents dine:Allergen conf=0.9
CREATE TABLE menu_item_allergens (
    menu_item_id  uuid NOT NULL REFERENCES menu_items (id) ON DELETE CASCADE,
    allergen      allergen NOT NULL,
    kind          allergen_kind NOT NULL,
    recorded_by   text NOT NULL,
    recorded_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (menu_item_id, allergen)
);

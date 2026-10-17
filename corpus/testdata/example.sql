-- @gloss represents zoo:Foal
CREATE TABLE foals (id uuid PRIMARY KEY, dam_id uuid, sire_id uuid);

/* @gloss represents zoo:Mare */
CREATE TABLE mares (
  id uuid PRIMARY KEY
);

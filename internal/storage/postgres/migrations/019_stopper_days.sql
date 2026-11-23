-- A stopper may have a duration in days. When it is set on a position, the end date («по») becomes
-- the start date («с») plus these days, or stays as it was when the position already ends earlier
-- (for example the promotion ends in 1C). NULL keeps an open period. The manager may change the period
-- afterwards; a new duration applies to later assignments only.
ALTER TABLE stoppers ADD COLUMN days SMALLINT CHECK (days BETWEEN 1 AND 365);
-- «Новинка» lasts 4 weeks.
UPDATE stoppers SET days=28 WHERE lower(name)='новинка' AND days IS NULL;

-- See the sqlite counterpart: payments built only from one-off lines were
-- stored without a concept, so they are filled in from their first line.
-- "First" here is the lowest concept id rather than the lowest line position:
-- on a charge whose lines carry different concepts the two can disagree, and
-- picking either one is a guess about a row nobody can reconstruct anyway.
UPDATE payments p
JOIN (
    SELECT pi.payment_id, MIN(pi.concept_id) AS concept_id
    FROM payment_items pi
    WHERE pi.concept_id IS NOT NULL
    GROUP BY pi.payment_id
) first_line ON first_line.payment_id = p.id
SET p.concept_id = first_line.concept_id
WHERE p.concept_id IS NULL;

-- Charges made only of one-off lines never copied a concept onto the payment
-- row, so the payments page listed every installation with an empty concept
-- column. The code no longer does that, but the rows already recorded stay
-- blank unless they are filled in from the lines they were built from.
UPDATE payments
SET concept_id = (
    SELECT pi.concept_id
    FROM payment_items pi
    WHERE pi.payment_id = payments.id AND pi.concept_id IS NOT NULL
    ORDER BY pi.position, pi.id
    LIMIT 1
)
WHERE concept_id IS NULL
  AND EXISTS (
    SELECT 1 FROM payment_items pi
    WHERE pi.payment_id = payments.id AND pi.concept_id IS NOT NULL
  );

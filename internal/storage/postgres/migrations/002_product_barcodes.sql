CREATE TABLE product_barcodes (
 product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
 position INTEGER NOT NULL CHECK (position >= 0),
 barcode TEXT NOT NULL,
 unit TEXT NOT NULL,
 ratio DOUBLE PRECISION NOT NULL,
 is_base BOOLEAN NOT NULL,
 PRIMARY KEY (product_id, position)
);
CREATE INDEX idx_product_barcodes_barcode ON product_barcodes(barcode);

INSERT INTO product_barcodes (product_id,position,barcode,unit,ratio,is_base)
SELECT p.id, (b.ordinality - 1)::integer,
 COALESCE(b.value->>'barcode',''), COALESCE(b.value->>'unit',''),
 COALESCE((b.value->>'ratio')::double precision,0),
 COALESCE((b.value->>'isBase')::boolean,false)
FROM products p CROSS JOIN LATERAL jsonb_array_elements(p.barcodes) WITH ORDINALITY AS b(value,ordinality);

ALTER TABLE products DROP COLUMN barcodes;

CREATE TABLE today_price_changes (
    store_id UUID NOT NULL,
    product_id UUID NOT NULL,
    price NUMERIC NOT NULL,
    is_promo BOOLEAN NOT NULL
);

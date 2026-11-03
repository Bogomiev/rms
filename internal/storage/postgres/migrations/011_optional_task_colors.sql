-- An empty color means the client uses its theme's standard card background.
-- Existing colors and defaults remain unchanged.
ALTER TABLE task_types
 DROP CONSTRAINT task_types_color_check,
 DROP CONSTRAINT task_types_color_dark_check,
 ADD CONSTRAINT task_types_color_check CHECK (color = '' OR color ~ '^#[0-9a-fA-F]{6}$'),
 ADD CONSTRAINT task_types_color_dark_check CHECK (color_dark = '' OR color_dark ~ '^#[0-9a-fA-F]{6}$');

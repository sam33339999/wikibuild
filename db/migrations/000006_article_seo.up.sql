-- Editable SEO / social fields (v1.1 S1). Empty string = use automatic fallbacks.
ALTER TABLE articles ADD COLUMN seo_title TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN summary TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN meta_description TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN cover_image_url TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN og_image_url TEXT NOT NULL DEFAULT '';

UPDATE forms SET settings = settings - 'rendererVersion' WHERE settings ? 'rendererVersion';
DROP TABLE IF EXISTS form_slug_aliases;

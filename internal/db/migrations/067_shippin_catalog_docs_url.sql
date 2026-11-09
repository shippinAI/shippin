-- A catalog entry's documentation link pointed into one site's private notes, and resolved nowhere else.
UPDATE catalog_services SET docs_url = '' WHERE docs_url LIKE '%/SHIPPIN-CLOUD/%';

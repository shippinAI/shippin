-- Tier ladder (the operator 2026-10-18 15:59 PDT): micro, single, dual, then 1, 2, 4, 8 boundaries, custom.
-- Sizes live in deploy/hostd.json plans.<id> (tiers/01-tiers.md: "All values become plans.<plan> keys");
-- this table only carries what the catalog sells: label, rank, price, public, boundary count.
-- DATA, not code (01-philosophy-rules "Nothing hardcoded ... prices"): editable in Plans & tiers, never
-- overwritten here. Prices are starting guesses, market-aligned to tiers/01 comparables
-- (Hetzner CX22-class, Codespaces 4-core/16 GB, 8-core/32 GB) ⚠ GUESS, for the operator to set.
-- micro/single/dual = one boundary VM of the tiers/01 micro/standard/pro size ⚠ interpretation.
INSERT INTO plans(id,label,description,rank,price_cents,public,boundaries) VALUES
 ('micro','Micro','One small private server (2 vCPU / 4 GiB) for one project.',100,900,true,1),
 ('single','Single','One private server (4 vCPU / 16 GiB) for a few projects.',110,2900,true,1),
 ('dual','Dual','One large private server (8 vCPU / 32 GiB) for big monorepos.',120,5900,true,1),
 ('b1','1 boundary','One dedicated 16 vCPU / 32 GiB server.',130,9900,true,1),
 ('b2','2 boundaries','Two dedicated servers; projects are placed where there is room.',140,19500,true,2),
 ('b4','4 boundaries','Four dedicated servers.',150,38500,true,4),
 ('b8','8 boundaries','Eight dedicated servers.',160,75900,true,8),
 ('custom','Custom','Contracted sizes and boundary count.',170,0,false,0),
 ('founder','Founder','Platform founder: one 16 vCPU / 32 GiB boundary (devspace).',5,0,false,1)
ON CONFLICT (id) DO NOTHING;

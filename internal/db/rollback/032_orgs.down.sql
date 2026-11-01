-- Manual rollback of 029_orgs.sql. Drops the org split tables (support grant history included;
-- export audit_log first if it must be kept: grants are also recorded there).
DROP TABLE IF EXISTS support_grants;
DROP TABLE IF EXISTS staff_assignments;
DROP TABLE IF EXISTS folders;
DROP TABLE IF EXISTS orgs;

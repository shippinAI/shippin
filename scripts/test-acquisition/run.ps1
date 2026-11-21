param([ValidateSet('setup','bootstrap','prepare','verify','baseline','migrate','resume','restore','persistence','tombstone')][string]$Phase, [string]$PrivateFixturePath=$env:ACQUISITION_FIXTURE_PATH, [string]$PlaywrightRoot=$env:ACQUISITION_TEST_PLAYWRIGHT_ROOT, [switch]$AllowLegacyUnlabeledFixtureSecrets, [ValidatePattern('^[a-zA-Z0-9-]+[.]json$')][string]$BaselineFileName='separated-baseline.json')
$ErrorActionPreference='Stop'
if(!$PrivateFixturePath -or !$PlaywrightRoot){throw 'Explicit private fixture directory and installed Playwright root required'}
$private=[IO.Path]::GetFullPath($PrivateFixturePath)
if(!(Test-Path (Join-Path $PlaywrightRoot 'node_modules/playwright/package.json'))){throw 'Installed Playwright not found'}
$env:ACQUISITION_TEST_ISOLATED='1'
$env:ACQUISITION_TEST_ORIGIN='https://127.0.0.1:25444'
$env:ACQUISITION_TEST_SIGNUP_ORIGIN='https://127.0.0.1:25443'
$env:ACQUISITION_TEST_SELF_SIGNED='1'
$env:ACQUISITION_TEST_PASSWORD_FILE=Join-Path $private 'password'
$env:ACQUISITION_TEST_SETUP_CODE_FILE=Join-Path $private 'setup-code'
$env:ACQUISITION_TEST_IDS_FILE=Join-Path $private 'fixture-ids.json'
$env:ACQUISITION_TEST_HISTORY_FILE=Join-Path $private 'fixture-record.json'
$env:ACQUISITION_TEST_BASELINE_FILE=Join-Path $private $BaselineFileName
$env:ACQUISITION_TEST_RECORD_FILE=Join-Path $private 'separated-record.json'
$env:ACQUISITION_TEST_ORIGINAL_SIGNER_FILE=Join-Path $private 'admin-signer'
$env:ACQUISITION_TEST_SIGNER_FILE=Join-Path $private 'export-signer'
$env:ACQUISITION_TEST_SOURCE_CA_FILE=Join-Path $private 'separated-admin-cert'
$env:ACQUISITION_TEST_PLAYWRIGHT_ROOT=[IO.Path]::GetFullPath($PlaywrightRoot)
function Assert-FixtureOwnership {
 if($env:SHIPPIN_ACQUISITION_DISPOSABLE -ne '1'){throw 'Explicit disposable setup opt-in required'}
 $db='shippin-acquisition-acceptance-source-db-1'
 $inspect=& docker inspect $db | ConvertFrom-Json
 if($LASTEXITCODE -ne 0 -or $inspect.Count -ne 1){throw 'Exact existing fixture database required'}
 if($inspect[0].Config.Labels.'com.docker.compose.project' -ne 'shippin-acquisition-acceptance' -or $inspect[0].Config.Labels.'com.docker.compose.service' -ne 'source-db'){throw 'Fixture ownership mismatch'}
 if(!($inspect[0].Mounts | Where-Object {$_.Name -eq 'shippin-acquisition-acceptance_source-db' -and $_.Destination -eq '/var/lib/postgresql/data'})){throw 'Unexpected fixture data volume'}
 $volume=& docker volume inspect shippin-acquisition-acceptance_source-secrets | ConvertFrom-Json
 if($LASTEXITCODE -ne 0 -or $volume.Count -ne 1){throw 'Existing owned secrets volume required'}
 $labels=$volume[0].Labels
 if(!$labels -or @($labels.PSObject.Properties).Count -eq 0) {
  if(!$AllowLegacyUnlabeledFixtureSecrets){throw 'Preserved legacy fixture secrets require explicit unlabeled-volume opt-in'}
 } elseif($labels.'com.docker.compose.project' -ne 'shippin-acquisition-acceptance' -or $labels.'com.docker.compose.volume' -ne 'source-secrets') {throw 'Secrets volume labels mismatch'}

 foreach($service in @('admin','signup')) {
  $serviceInfo=& docker inspect "shippin-acquisition-acceptance-$service-1" | ConvertFrom-Json
  if($LASTEXITCODE -ne 0 -or $serviceInfo.Count -ne 1 -or $serviceInfo[0].Config.Labels.'com.docker.compose.project' -ne 'shippin-acquisition-acceptance' -or $serviceInfo[0].Config.Labels.'com.docker.compose.service' -ne $service){throw 'Original fixture service ownership mismatch'}
  if(!($serviceInfo[0].Mounts | Where-Object {$_.Name -eq 'shippin-acquisition-acceptance_source-secrets' -and $_.Destination -eq '/secrets' -and !$_.RW})){throw 'Original fixture service secrets mount mismatch'}
 }

 $identity=& docker exec $db psql -U shippin_test -d shippin_signup_disposable -At -v ON_ERROR_STOP=1 -c "SELECT current_database()='shippin_signup_disposable' AND current_user='shippin_test' AND EXISTS(SELECT 1 FROM shippin_marketing.scopes WHERE scope='container/signup') AND (SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname='shippin_marketing')=6"
 if($LASTEXITCODE -ne 0 -or "$identity".Trim() -ne 't'){throw 'Synthetic source identity/scope/schema mismatch'}
}
if($Phase -in @('setup','migrate','resume')) {Assert-FixtureOwnership}
if($Phase -in @('migrate','resume')) {
 & docker run --rm --network container:shippin-acquisition-acceptance-source-db-1 --read-only --cap-drop ALL --security-opt no-new-privileges -v shippin-acquisition-acceptance_source-secrets:/secrets:ro -e SHIPPIN_ACQUISITION_DISPOSABLE=1 shippin-acquisition-fixture:qualification probe
 if($LASTEXITCODE -ne 0){throw 'Separated exporter readiness/old-key denial probe failed'}
}
if($Phase -eq 'setup') {
 $db='shippin-acquisition-acceptance-source-db-1'
 $sql=@"
BEGIN;
DO `$`$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='shippin_export_fixture') THEN
  RAISE EXCEPTION 'Export role already exists; inspect exact state before rerunning setup';
 END IF;
END `$`$;
CREATE ROLE shippin_export_fixture LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
GRANT CONNECT ON DATABASE shippin_signup_disposable TO shippin_export_fixture;
GRANT USAGE ON SCHEMA shippin_marketing TO shippin_export_fixture;
GRANT SELECT ON shippin_marketing.schema_version,shippin_marketing.scopes,shippin_marketing.leads,shippin_marketing.tombstone_auth TO shippin_export_fixture;
ALTER ROLE shippin_export_fixture SET default_transaction_read_only=on;
DO `$`$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='shippin_export_fixture'))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_tables WHERE schemaname='shippin_marketing' AND (pg_catalog.has_table_privilege('shippin_export_fixture',quote_ident(schemaname)||'.'||quote_ident(tablename),'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER,MAINTAIN') OR pg_catalog.has_any_column_privilege('shippin_export_fixture',quote_ident(schemaname)||'.'||quote_ident(tablename),'INSERT,UPDATE,REFERENCES'))) THEN
 RAISE EXCEPTION 'Unsafe exporter grants'; END IF;
END `$`$;
COMMIT;
"@
 $sql | & docker exec -i $db psql -U shippin_test -d shippin_signup_disposable -v ON_ERROR_STOP=1
 if($LASTEXITCODE -ne 0){throw 'Fixture role preparation failed; no automatic repair'}
 foreach($operation in @('exporter-files','renew-certs')) {
  & docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges -v shippin-acquisition-acceptance_source-secrets:/secrets -e SHIPPIN_ACQUISITION_DISPOSABLE=1 shippin-acquisition-fixture:qualification $operation
  if($LASTEXITCODE -ne 0){throw 'Fixture material preparation failed; inspect without replacing secrets'}
 }
 Write-Output 'PASS bounded fixture role/material preparation; copy synthetic exporter signer and CA privately before baseline'
 exit 0
}
if($Phase -eq 'bootstrap') {
  $logs=cmd /c "docker logs shippin-acquisition-acceptance-panel-1 2>&1" | Out-String
  $matches=[regex]::Matches($logs,'setup_code=([^\s]+)')
  if($matches.Count -lt 1){throw 'Fixture setup code not available'}
  [IO.File]::WriteAllText($env:ACQUISITION_TEST_SETUP_CODE_FILE,$matches[$matches.Count-1].Groups[1].Value.Trim('"'))
  $logs=$null; $matches=$null
}
Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..')))
try {node web/scripts/test-acquisition-integration.mjs $Phase;exit $LASTEXITCODE} finally {Pop-Location}

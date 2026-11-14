param([ValidateSet('bootstrap','prepare','verify','persistence','tombstone')][string]$Phase, [string]$PrivateFixturePath=$env:ACQUISITION_FIXTURE_PATH, [string]$PlaywrightRoot=$env:ACQUISITION_TEST_PLAYWRIGHT_ROOT)
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
$env:ACQUISITION_TEST_RECORD_FILE=Join-Path $private 'fixture-record.json'
$env:ACQUISITION_TEST_SIGNER_FILE=Join-Path $private 'admin-signer'
$env:ACQUISITION_TEST_SOURCE_CA_FILE=Join-Path $private 'admin-cert'
$env:ACQUISITION_TEST_PLAYWRIGHT_ROOT=[IO.Path]::GetFullPath($PlaywrightRoot)
if($Phase -eq 'bootstrap') {
  $logs=cmd /c "docker logs shippin-acquisition-acceptance-panel-1 2>&1" | Out-String
  $matches=[regex]::Matches($logs,'setup_code=([^\s]+)')
  if($matches.Count -lt 1){throw 'Fixture setup code not available'}
  [IO.File]::WriteAllText($env:ACQUISITION_TEST_SETUP_CODE_FILE,$matches[$matches.Count-1].Groups[1].Value.Trim('"'))
  $logs=$null; $matches=$null
}
Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..')))
try {node web/scripts/test-acquisition-integration.mjs $Phase;exit $LASTEXITCODE} finally {Pop-Location}

# Verify that the Microsoft Edge WebView2 runtime is installed; install the signed Evergreen runtime if it is missing.
# Used by CI runners only. TestDesk never installs WebView2 on a user's machine.
$ErrorActionPreference = 'Stop'
$keys = @(
    'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
    'HKCU:\Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
)
foreach ($key in $keys) {
    $version = (Get-ItemProperty -LiteralPath $key -Name pv -ErrorAction SilentlyContinue).pv
    if ($version -match '^\d+\.\d+\.\d+\.\d+$' -and [version]$version -gt [version]'0.0.0.0') { exit 0 }
}
$installer = Join-Path $env:RUNNER_TEMP 'MicrosoftEdgeWebview2Setup.exe'
Invoke-WebRequest 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $installer
$signature = Get-AuthenticodeSignature -LiteralPath $installer
if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation') { throw 'Invalid WebView2 installer signature' }
$process = Start-Process -FilePath $installer -ArgumentList '/silent', '/install' -WindowStyle Hidden -Wait -PassThru
if ($process.ExitCode -ne 0) { throw "WebView2 installation failed: $($process.ExitCode)" }

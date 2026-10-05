# Deploys deploy/main.bicep. Run from the repository root after `az login` and `neon link`.
#
# The three secrets never appear on a command line: the Neon connection string
# contains '&', which cmd (behind az.cmd) treats as a command separator. They
# go through a parameters file that exists only while az runs.
param(
	# The image to run. Defaults to the one the server runs now, so an
	# infrastructure-only deploy never rolls the app back.
	[string]$Image,
	[string]$ResourceGroup = 'where-to-bird-today',
	[string]$Location = 'germanywestcentral',
	[string]$GhcrUsername = 'Kontora20',
	[Parameter(Mandatory)][string]$BudgetEmail
)
$ErrorActionPreference = 'Stop'

if ((az group exists --name $ResourceGroup) -eq 'false') {
	az group create --name $ResourceGroup --location $Location --output none
}
if (-not $Image) {
	$Image = az containerapp list --resource-group $ResourceGroup --query "[?name=='wtbt-server'].properties.template.containers[0].image | [0]" -o tsv
	if (-not $Image) { throw 'No server is running yet: pass -Image ghcr.io/kontora20/where-to-bird-today:<sha>' }
}

$databaseUrl = neon connection-string --database-name neondb
if ($databaseUrl -notlike 'postgres*' -or $databaseUrl -like '*-pooler*') { throw 'neon connection-string did not return a direct connection string' }
$ebirdApiKey = (Select-String -Path .env -Pattern '^EBIRD_API_KEY=(.+)$').Matches[0].Groups[1].Value.Trim()
$ghcrToken = [System.Net.NetworkCredential]::new('', (Read-Host 'GitHub token with only read:packages' -AsSecureString)).Password

$parameters = @{
	'$schema'      = 'https://schema.management.azure.com/schemas/2019-04-01/deploymentParameters.json#'
	contentVersion = '1.0.0.0'
	parameters     = @{
		image            = @{ value = $Image }
		ghcrUsername     = @{ value = $GhcrUsername }
		ghcrToken        = @{ value = $ghcrToken }
		databaseUrl      = @{ value = $databaseUrl }
		ebirdApiKey      = @{ value = $ebirdApiKey }
		budgetEmail      = @{ value = $BudgetEmail }
		adminPrincipalId = @{ value = (az ad signed-in-user show --query id -o tsv) }
	}
}
$file = Join-Path $env:TEMP "wtbt-params-$([guid]::NewGuid()).json"
try {
	# WriteAllText writes UTF-8 with no byte-order mark, which az needs.
	[System.IO.File]::WriteAllText($file, ($parameters | ConvertTo-Json -Depth 5))
	az deployment group create --resource-group $ResourceGroup --template-file deploy/main.bicep --parameters "@$file" --query properties.outputs -o json
	if ($LASTEXITCODE -ne 0) { throw "deployment failed with exit code $LASTEXITCODE" }
}
finally {
	Remove-Item -Path $file -Force -ErrorAction SilentlyContinue
}

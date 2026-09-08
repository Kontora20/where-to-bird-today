# Loads .env into real process environment variables, then runs the server.
# This does not change what cmd/server reads: it still only sees plain env
# vars via os.Getenv, exactly as it would in production -- this script just
# automates the shell setup step documented in README.md's Quick start.
if (-not (Test-Path .env)) {
	Write-Error ".env not found. Copy .env.example to .env and fill it in first."
	exit 1
}

Get-Content .env | Where-Object { $_ -match '^\s*[^#].*=' } | ForEach-Object {
	$name, $value = $_ -split '=', 2
	Set-Item "env:$($name.Trim())" $value.Trim()
}

go run ./cmd/server

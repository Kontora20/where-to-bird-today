# Deploying

The database is on Neon's free plan (1 GB, PostGIS, suspends after 5 minutes idle). `main.bicep`
creates the rest in one Azure resource group in Frankfurt, next to Neon's AWS eu-central-1 region: the
server and OSRM as Container Apps that scale to zero, the daily `recent`, weekly `hotspots` and manual
`migrate` jobs, Key Vault, a file share for the road graph, a log workspace capped at 100 MB a day, and
a monthly budget alert.

Secrets never touch a file. `az` asks for the secure parameters when they are left out, Bicep writes them
to Key Vault, and the apps read them from there through a managed identity.

## First deploy

Needs Azure CLI (`winget install Microsoft.AzureCLI`) and Docker.

1. Create a Neon project in **AWS Europe Central 1 (Frankfurt)** with Postgres 17, and a database named
   `wheretobird`. Copy its connection string with **connection pooling turned off**.

2. Copy the local database into Neon (653 MB). The dump creates the `postgis` and `unaccent` extensions
   itself. Paste the connection string when the command asks, so it stays out of your shell history.

   ```powershell
   docker exec wtbt-postgres pg_dump -U birder -d wheretobird -Fc -f /tmp/wtbt.dump
   $neon = Read-Host 'Neon connection string'
   docker exec wtbt-postgres pg_restore --no-owner --no-privileges -d $neon /tmp/wtbt.dump
   ```

3. Push the image. The repository is private, so the image is too. Use a classic GitHub token with
   `write:packages` to push, and a second one with only `read:packages` for Azure to pull.

   ```bash
   docker build -t ghcr.io/kontora20/where-to-bird-today:$(git rev-parse --short HEAD) .
   docker push ghcr.io/kontora20/where-to-bird-today:$(git rev-parse --short HEAD)
   ```

4. Create the resource group and deploy. `az` prompts for `ghcrToken`, `databaseUrl` and `ebirdApiKey`.

   ```bash
   az login
   az group create --name where-to-bird-today --location germanywestcentral
   az deployment group create --resource-group where-to-bird-today --template-file deploy/main.bicep --parameters image=ghcr.io/kontora20/where-to-bird-today:<sha> ghcrUsername=<github user> budgetEmail=<email> adminPrincipalId=$(az ad signed-in-user show --query id -o tsv)
   ```

   If the first deploy fails with a Key Vault access error, run it again: the managed identity's role
   can take a minute to apply.

5. Upload the road graph (only the `baltics.osrm*` files, 1.3 GB). Take the account name from the
   deployment output `storageAccount`.

   ```bash
   az storage file upload-batch --account-name <storageAccount> --destination osrm --source data/osm --pattern "baltics.osrm*"
   ```

## Later deploys

Push a new image and run step 4 again with the new tag. When it adds a migration, run it:

```bash
az containerapp job start --resource-group where-to-bird-today --name wtbt-migrate
```

## Monthly eBird export

Read the connection string from Key Vault, set it as `DATABASE_URL`, and run the README's "Loading eBird
data" steps against it, ending with `ingest-ebird-recent`:

```bash
az keyvault secret show --vault-name <keyVault> --name database-url --query value -o tsv
```

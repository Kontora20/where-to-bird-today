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

## Deploys from GitHub

After CI passes on `main`, `.github/workflows/deploy.yml` builds and pushes the image, runs the `migrate`
job, then moves the server and the scheduled jobs to the new image. It only changes images. Changes to
`main.bicep` are still deployed by hand with step 4, and that step needs the image that is running now,
or it puts the old one back:

```bash
az containerapp show --resource-group where-to-bird-today --name wtbt-server --query "properties.template.containers[0].image" -o tsv
```

One-time setup, so the workflow can sign in to Azure with no stored password. GitHub vouches for each run
from the `production` environment, and Azure trusts that alone:

```bash
app=$(az ad app create --display-name where-to-bird-today-deploy --query appId -o tsv)
az ad sp create --id $app
az ad app federated-credential create --id $app --parameters '{"name":"production","issuer":"https://token.actions.githubusercontent.com","subject":"repo:Kontora20/where-to-bird-today:environment:production","audiences":["api://AzureADTokenExchange"]}'
az role assignment create --assignee $app --role Contributor --scope $(az group show --name where-to-bird-today --query id -o tsv)
gh variable set AZURE_CLIENT_ID --body $app
gh variable set AZURE_TENANT_ID --body $(az account show --query tenantId -o tsv)
gh variable set AZURE_SUBSCRIPTION_ID --body $(az account show --query id -o tsv)
```

The workflow skips itself until `AZURE_CLIENT_ID` is set. GitHub creates the `production` environment
on the first run; add yourself as a required reviewer there to approve each deploy. The three IDs are not
secrets: without the federated credential they let no one in.

## Monthly eBird export

Read the connection string from Key Vault, set it as `DATABASE_URL`, and run the README's "Loading eBird
data" steps against it, ending with `ingest-ebird-recent`:

```bash
az keyvault secret show --vault-name <keyVault> --name database-url --query value -o tsv
```

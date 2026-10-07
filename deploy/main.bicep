// Azure resources for Where to Bird Today, in one resource group. The
// database is on Neon, outside Azure. Deploy with deploy/README.md. Secrets are passed as secure parameters at deploy
// time and stored only in Key Vault; the apps read them from there through a
// managed identity.

@description('Azure region. Frankfurt, next to the Neon database in AWS eu-central-1.')
param location string = 'germanywestcentral'

@description('Prefix for every resource name.')
param namePrefix string = 'wtbt'

@description('Container image of the server and the cmd/ tools, for example ghcr.io/kontora20/where-to-bird-today:<git sha>.')
param image string

@description('GitHub user that owns the classic token below.')
param ghcrUsername string

@secure()
@description('Classic GitHub token with only the read:packages scope, so Azure can pull the private image.')
param ghcrToken string

@secure()
@description('Neon connection string to the direct host (not -pooler), postgres://...?sslmode=require.')
param databaseUrl string

@secure()
@description('eBird API key, used by the scheduled jobs.')
param ebirdApiKey string

@description('Your Entra object ID (az ad signed-in-user show --query id -o tsv), so you can read and change the secrets. Empty for none.')
param adminPrincipalId string = ''

@description('Email that receives the budget alerts.')
param budgetEmail string

@description('Monthly budget in the billing currency.')
param monthlyBudget int = 25

@description('First day of the first budget month. Cannot be moved later than the existing start date once created.')
param budgetStartDate string = '2026-10-01'

var osrmImage = 'ghcr.io/project-osrm/osrm-backend@sha256:8a1b1bc938412f15f9b5b32d794c4ec6bf4a85dfbbabfa0a014b70b187edb53b'
var keyVaultSecretsUserRole = '4633458b-17de-408a-b874-0445c86b69e6'
var keyVaultSecretsOfficerRole = 'b86a8fe4-44ce-4948-aee5-eccb2c155cd7'

resource logs 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name: '${namePrefix}-logs'
  location: location
  properties: {
    sku: { name: 'PerGB2018' }
    retentionInDays: 30
    // Stops ingestion for the day at 100 MB, so a log flood cannot run up a bill.
    workspaceCapping: { dailyQuotaGb: json('0.1') }
  }
}

// The dashboard over the logs above: Azure portal > Monitor > Workbooks. Free; it only runs queries.
resource workbook 'Microsoft.Insights/workbooks@2022-04-01' = {
  name: guid(resourceGroup().id, '${namePrefix}-overview')
  location: location
  kind: 'shared'
  properties: {
    displayName: 'Where to Bird Today'
    category: 'workbook'
    sourceId: logs.id
    serializedData: replace(loadTextContent('workbook.json'), '@@LOGS_ID@@', logs.id)
  }
}

resource identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: '${namePrefix}-identity'
  location: location
}

resource vault 'Microsoft.KeyVault/vaults@2023-07-01' = {
  // Key Vault names are global, so the name carries a suffix unique to the resource group.
  name: '${namePrefix}-kv-${take(uniqueString(resourceGroup().id), 6)}'
  location: location
  properties: {
    tenantId: subscription().tenantId
    sku: { family: 'A', name: 'standard' }
    enableRbacAuthorization: true
    softDeleteRetentionInDays: 7
  }
}

resource vaultReader 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(vault.id, identity.id, keyVaultSecretsUserRole)
  scope: vault
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', keyVaultSecretsUserRole)
    principalId: identity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

resource vaultAdmin 'Microsoft.Authorization/roleAssignments@2022-04-01' = if (!empty(adminPrincipalId)) {
  name: guid(vault.id, adminPrincipalId, keyVaultSecretsOfficerRole)
  scope: vault
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', keyVaultSecretsOfficerRole)
    principalId: adminPrincipalId
    principalType: 'User'
  }
}

resource databaseUrlSecret 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: vault
  name: 'database-url'
  properties: { value: databaseUrl }
}

resource ebirdSecret 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: vault
  name: 'ebird-api-key'
  properties: { value: ebirdApiKey }
}

resource ghcrSecret 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: vault
  name: 'ghcr-token'
  properties: { value: ghcrToken }
}

// Holds the OSRM road graph (1.3 GB), uploaded once from data/osm.
resource storage 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: '${namePrefix}osrm${take(uniqueString(resourceGroup().id), 6)}'
  location: location
  sku: { name: 'Standard_LRS' }
  kind: 'StorageV2'
  properties: {
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
    supportsHttpsTrafficOnly: true
  }
}

resource osrmShare 'Microsoft.Storage/storageAccounts/fileServices/shares@2023-05-01' = {
  name: '${storage.name}/default/osrm'
  properties: { shareQuota: 5 }
}

resource environment 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name: '${namePrefix}-env'
  location: location
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logs.properties.customerId
        sharedKey: logs.listKeys().primarySharedKey
      }
    }
  }
}

resource osrmStorage 'Microsoft.App/managedEnvironments/storages@2024-03-01' = {
  parent: environment
  name: 'osrm'
  properties: {
    azureFile: {
      accountName: storage.name
      accountKey: storage.listKeys().keys[0].value
      shareName: 'osrm'
      accessMode: 'ReadOnly'
    }
  }
}

var registries = [
  {
    server: 'ghcr.io'
    username: ghcrUsername
    passwordSecretRef: 'ghcr-token'
  }
]

var vaultSecrets = [
  { name: 'database-url', keyVaultUrl: databaseUrlSecret.properties.secretUri, identity: identity.id }
  { name: 'ebird-api-key', keyVaultUrl: ebirdSecret.properties.secretUri, identity: identity.id }
  { name: 'ghcr-token', keyVaultUrl: ghcrSecret.properties.secretUri, identity: identity.id }
]

var jobEnv = [
  { name: 'DATABASE_URL', secretRef: 'database-url' }
  { name: 'EBIRD_API_KEY', secretRef: 'ebird-api-key' }
]

// Internal only: reachable from the server inside the environment, never from the internet.
resource osrm 'Microsoft.App/containerApps@2024-03-01' = {
  name: '${namePrefix}-osrm'
  location: location
  dependsOn: [osrmStorage]
  properties: {
    managedEnvironmentId: environment.id
    configuration: {
      ingress: { external: false, targetPort: 5000, transport: 'http' }
    }
    template: {
      containers: [
        {
          name: 'osrm'
          image: osrmImage
          command: ['osrm-routed', '--algorithm', 'ch', '--mmap', 'on', '--max-table-size', '1000', '/data/baltics.osrm']
          // OSRM needs about 340 MiB and slows badly below 512 MiB; 1 GiB is
          // the memory that comes with 0.5 vCPU on the consumption plan.
          resources: { cpu: json('0.5'), memory: '1Gi' }
          volumeMounts: [{ volumeName: 'osrm', mountPath: '/data' }]
        }
      ]
      volumes: [{ name: 'osrm', storageType: 'AzureFile', storageName: 'osrm' }]
      scale: { minReplicas: 0, maxReplicas: 1 }
    }
  }
}

resource server 'Microsoft.App/containerApps@2024-03-01' = {
  name: '${namePrefix}-server'
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: { '${identity.id}': {} }
  }
  dependsOn: [vaultReader]
  properties: {
    managedEnvironmentId: environment.id
    configuration: {
      ingress: { external: true, targetPort: 8080, transport: 'http', allowInsecure: false }
      registries: registries
      secrets: vaultSecrets
    }
    template: {
      containers: [
        {
          name: 'server'
          image: image
          resources: { cpu: json('0.25'), memory: '0.5Gi' }
          env: [
            { name: 'DATABASE_URL', secretRef: 'database-url' }
            { name: 'OSRM_URL', value: 'http://${osrm.name}' }
          ]
          probes: [
            { type: 'Liveness', httpGet: { path: '/health', port: 8080 } }
            { type: 'Readiness', httpGet: { path: '/ready', port: 8080 } }
          ]
        }
      ]
      // One replica at most: the bill cannot grow with traffic, whatever a bot does.
      scale: { minReplicas: 0, maxReplicas: 1 }
    }
  }
}

var jobs = [
  { name: 'recent', command: '/app/ingest-ebird-recent', args: [], cron: '0 4 * * *' }
  { name: 'hotspots', command: '/app/ingest-ebird-hotspots', args: [], cron: '0 3 * * 1' }
  { name: 'migrate', command: '/app/migrate', args: ['up'], cron: '' }
]

// Scheduled jobs run the same image with another command. migrate has no
// schedule: start it by hand after a deploy that adds a migration.
resource job 'Microsoft.App/jobs@2024-03-01' = [
  for item in jobs: {
    name: '${namePrefix}-${item.name}'
    location: location
    identity: {
      type: 'UserAssigned'
      userAssignedIdentities: { '${identity.id}': {} }
    }
    dependsOn: [vaultReader]
    properties: {
      environmentId: environment.id
      configuration: {
        triggerType: empty(item.cron) ? 'Manual' : 'Schedule'
        scheduleTriggerConfig: empty(item.cron) ? null : { cronExpression: item.cron, parallelism: 1, replicaCompletionCount: 1 }
        manualTriggerConfig: empty(item.cron) ? { parallelism: 1, replicaCompletionCount: 1 } : null
        replicaTimeout: 1800
        replicaRetryLimit: 1
        registries: registries
        secrets: vaultSecrets
      }
      template: {
        containers: [
          {
            name: item.name
            image: image
            command: [item.command]
            args: item.args
            resources: { cpu: json('0.25'), memory: '0.5Gi' }
            env: jobEnv
          }
        ]
      }
    }
  }
]

resource budget 'Microsoft.Consumption/budgets@2023-11-01' = {
  name: '${namePrefix}-monthly'
  properties: {
    category: 'Cost'
    amount: monthlyBudget
    timeGrain: 'Monthly'
    timePeriod: { startDate: budgetStartDate }
    notifications: {
      actual80: {
        enabled: true
        operator: 'GreaterThanOrEqualTo'
        threshold: 80
        thresholdType: 'Actual'
        contactEmails: [budgetEmail]
      }
      forecast100: {
        enabled: true
        operator: 'GreaterThanOrEqualTo'
        threshold: 100
        thresholdType: 'Forecasted'
        contactEmails: [budgetEmail]
      }
    }
  }
}

output url string = 'https://${server.properties.configuration.ingress.fqdn}'
output storageAccount string = storage.name
output keyVault string = vault.name

local environment = std.extVar('environment');
local cluster = std.extVar('cluster');

local isLocalDev = environment == 'local_development';
local isDev = environment == 'development' || isLocalDev;
local isStaging = environment == 'staging';

local accountsHost = if isDev then
  'accounts.outreach-dev.com'
else if isStaging then
  'accounts.outreach-staging.com'
else
  'accounts.outreach.io';

local accountsInternalHost = if isDev then
  'outreach-accounts.outreach-accounts'
else
  accountsHost;

local baseURL = if isDev then
  'http://%s' % accountsHost
else
  'https://%s' % accountsHost;

{
  hostname: accountsHost,
  internalHostname: accountsInternalHost,
  serviceEnv: {
    OUTREACH_ACCOUNTS_BASE_URL: baseURL,
  },
}

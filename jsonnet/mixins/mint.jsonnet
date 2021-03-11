local ok = import 'kubernetes/outreach.libsonnet';

// TODO: Remove scim
local name = 'scim';
local environment = std.extVar('environment');
local bento = std.extVar('bento');
local cluster = std.extVar('cluster');
local namespace = std.extVar('namespace');
local resources = import './resources.libsonnet';

local isLocalDev = environment == 'local_development';
local isDev = environment == 'development' || isLocalDev;

local all = {
  svc_acct: ok.ServiceAccount('scim-svc', namespace) {
    metadata+: {
      annotations+: {
        'outreach.io/authn-v1-service-id': 'scim@outreach.cloud',
        'outreach.io/authn-v1-audience-ids': '',
        'outreach.io/authn-v1-can-impersonate-user': 'false',
        'outreach.io/authn-v1-permitted-user-scopes': '',
      },
    },
  },

  // Grant this service account permission to issue tokens that vouch for
  // its identity to other services.
  service_token_issue_role: ok.Role(name + '-service-token-role', app=name, namespace=namespace) {
    rules: [{
      apiGroups: [''],
      resources: ['serviceaccounts/token'],
      verbs: ['create'],
      resourceNames: [$.svc_acct.metadata.name],
    }],
  },
  service_token_issue_role_binding: ok.RoleBinding(name, app=name, namespace=namespace) {
    subjects_: [$.svc_acct],
    roleRef_: $.service_token_issue_role,
  },

  deployment+: {
    spec+: {
      template+: {
        spec+: {
          serviceAccountName: $.svc_acct.metadata.name,
          containers_+:: {
            default+: {
              volumeMounts_+:: {
                'config-authn-mint-volume': {
                  mountPath: '/run/config/outreach.io/authn_mint.yaml',
                  subPath: 'authn_mint.yaml',
                },
                'secret-authn-mint-volume': {
                  mountPath: '/run/secrets/outreach.io/mint-validator-payload',
                },
                'config-authn-flagship-volume': {
                  mountPath: '/run/config/outreach.io/authn_flagship.yaml',
                  subPath: 'authn_flagship.yaml',
                },
                'secret-authn-flagship-volume': {
                  mountPath: '/run/secrets/outreach.io/authn-flagship-payload',
                },
              },
            },
          },
          volumes_+:: {
            'config-authn-mint-volume': ok.ConfigMapVolume(ok.ConfigMap('config-authn-mint', namespace)),
            'secret-authn-mint-volume': (
              // Allow pod creation to proceed even if secret is unavailable.
              ok.SecretVolume(ok.Secret('mint-validator-payload', namespace)) { secret+: { optional: true } }
            ),
            'config-authn-flagship-volume': ok.ConfigMapVolume(ok.ConfigMap('config-authn-flagship', namespace)),
            'secret-authn-flagship-volume': (
              // Allow pod creation to proceed even if secret is unavailable.
              ok.SecretVolume(ok.Secret('authn-flagship-payload', namespace)) { secret+: { optional: true } }
            ),
          },
        },
      },
    },
  },

  // We pull in all the public keys for this environment.
  // The app will choose which of these to consider trustworthy.
  authn_mint_configmap: ok.ConfigMap('config-authn-mint', namespace) {
    local this = self,
    data_:: {
      Path: '/run/secrets/outreach.io/mint-validator-payload/validation_keys_jwks',
    },
    data: {
      'authn_mint.yaml': std.manifestYamlDoc(this.data_),
    },
  },
  authn_flagship_configmap: ok.ConfigMap('config-authn-flagship', namespace) {
    local this = self,
    data_:: {
      Path: '/run/secrets/outreach.io/authn-flagship-payload/internal_secret',
    },
    data: {
      'authn_flagship.yaml': std.manifestYamlDoc(this.data_),
    },
  },
};

local developmentResources = {
  // These secrets will be included in dev by default, they are fetched from vault.
  'mint-validator-payload': {
    apiVersion: 'ricoberger.de/v1alpha1',
    kind: 'VaultSecret',
    metadata: {
      name: 'mint-validator-payload',
      namespace: namespace,
    },
    spec: {
      path: 'deploy/mint/%(environment)s/validation/mint-validator-payload' % {
        environment: environment,
        bento: bento,
        cluster: cluster,
      },
      type: 'Opaque',
    },
  },
  'authn-flagship-payload': {
    apiVersion: 'ricoberger.de/v1alpha1',
    kind: 'VaultSecret',
    metadata: {
      name: 'authn-flagship-payload',
      namespace: namespace,
    },
    spec: {
      path: 'deploy/flagship-shared-secret/%s/authn-flagship-payload' % environment,
      type: 'Opaque',
    },
  },

  // This service accounts is used in e2e tests and is otherwise harmless.
  // It is configured with the permissions it will need to take on any user
  // identity and any scopes required to execute the tests.  It also has
  // permission to send requests to the service under test, of course.
  e2e_svc_account: ok.ServiceAccount('scim-e2e-client-svc', namespace) {
    metadata+: {
      annotations+: {
        'outreach.io/authn-v1-service-id': 'scim-e2e-client@outreach.cloud',
        'outreach.io/authn-v1-audience-ids': 'scim@outreach.cloud flagship@outreach.cloud flagship-internal@outreach.cloud',
        'outreach.io/authn-v1-can-impersonate-user': 'true',
        'outreach.io/authn-v1-permitted-user-scopes': 'AAA=',
      },
    },
  },
};

all + (if isDev then developmentResources else {})

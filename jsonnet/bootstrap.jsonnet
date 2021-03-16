local accountsMixin = import './mixins/accounts.env.jsonnet';
local mintMixin = import './mixins/mint.jsonnet';

// Outreach jsonnet-libs w/ some custom additions
local originalOk = import 'kubernetes/outreach.libsonnet';
local ok = originalOk {
  // VaultSecret creates a VaultSecret in the devenv
  VaultSecret(name, namespace): ok._Object('ricoberger.de/v1alpha1', 'VaultSecret', name=name, namespace=namespace) {
    spec+: {
      type: 'Opaque',
      path: error 'vaultsecret path is required',
    },
  },
};

// k8s cluster stuff
local environment = std.extVar('environment');
local bento = std.extVar('bento');
local cluster = std.extVar('cluster');
local namespace = std.extVar('namespace');

local isLocalDev = environment == 'local_development';
local isDev = environment == 'development' || isLocalDev;

{
  GenerateKubernetesObjects(name, reportingTeam, namespace='%s--%s' % [name, bento], vault_secrets=[]):: {
    local this = self,
    local sharedLabels = {
      repo: name,
      bento: bento,
      reporting_team: reportingTeam,
    },
    namespace: ok.Namespace(namespace) {
      metadata+: {
        annotations+: {
          'iam.amazonaws.com/permitted': '%s_service_role' % name,
        },
        labels+: sharedLabels,
      },
    },
    service: ok.Service(name, namespace) {
      target_pod:: this.deployment.spec.template,
      metadata+: {
        labels+: sharedLabels,
      },
      spec+: {
        local this = self,
        sessionAffinity: 'None',
        type: 'ClusterIP',
        ports_:: {
          grpc: {
            port: 5000,
            targetPort: 'grpc',
          },
          metrics: {
            port: 8000,
            targetPort: 'http-prom',
          },
          http: {
            port: 8080,
            targetPort: 'http',
          },
        },
        ports: ok.mapToNamedList(this.ports_),
      },
    },
    pdb: ok.PodDisruptionBudget(name, namespace) {
      metadata+: {
        labels: sharedLabels,
      },
      spec+: { maxUnavailable: 1 },
    },

    configmap: ok.ConfigMap('config', namespace) {
      local this = self,
      data_:: {},
      data: {
        // We use this.data_ to allow for ez merging in the override.
        ['%s.yaml' % name]: std.manifestYamlDoc(this.data_),
      },
    },
    trace_configmap: ok.ConfigMap('config-trace', namespace) {
      local this = self,
      data_:: {
        Honeycomb: {
          Enabled: true,
          APIHost: 'https://api.honeycomb.io',
          APIKey: {
            // NOTE: This is needed to be manually overriden due to how the keys were configured in vault.
            Path: if isLocalDev then '/run/secrets/outreach.io/dev-env/apiKey' else '/run/secrets/outreach.io/honeycomb/apiKey',
          },
          Dataset: if isDev then 'dev' else 'outreach',
          SamplePercent: if isDev then 100 else 1,
        },
      },
      data: {
        // We use this.data_ to allow for ez merging in the override.
        'trace.yaml': std.manifestYamlDoc(this.data_),
      },
    },
    // TODO(jaredallard): When k8s-deploy-resource supports gc
    // we should rename this...
    fflags_configmap: ok.ConfigMap('fflags-yaml', namespace) {
      local this = self,
      data_:: {
        apiKey: {
          Path: '/run/secrets/outreach.io/launchdarkly/sdk-key',
        },
        flagsToAdd: {
          bento: bento,
        },
      },
      data: {
        // We use this.data_ to allow for ez merging in the override.
        'fflags.yaml': std.manifestYamlDoc(this.data_),
      },
    },

    deployment: ok.Deployment(name, namespace) {
      metadata+: {
        labels+: sharedLabels,
      },
      spec+: {
        replicas: if (isDev || isLocalDev) then 1 else 2,
        template+: {
          metadata+: {
            labels+: sharedLabels,
            annotations+: {
              'iam.amazonaws.com/role': '%s_service_role' % name,
              // https://docs.datadoghq.com/integrations/openmetrics/
              ['ad.datadoghq.com/' + name + '.check_names']: '["openmetrics"]',
              ['ad.datadoghq.com/' + name + '.init_configs']: '[{}]',
              ['ad.datadoghq.com/' + name + '.instances']: std.manifestJsonEx([
                {
                  prometheus_url: 'http://%%host%%:' +
                                  this.deployment.spec.template.spec.containers_.default.ports_['http-prom'].containerPort +
                                  '/metrics',
                  namespace: name,
                  metrics: ['*'],
                  send_distribution_buckets: true,
                },
              ], '  '),
            },
          },
          spec+: {
            containers_:: {
              default: ok.Container(name) {
                image: 'gcr.io/outreach-docker/%s:%s' % [name, std.extVar('version')],

                // We don't want to ever pull the same tag multiple times.
                // In dev, this is replaced by sharing docker image cache with Kubernetes
                // so we also don't need to pull images.
                imagePullPolicy: 'IfNotPresent',
                volumeMounts_+:: {
                  // default configuration files
                  ['config-%s' % name]: {
                    mountPath: '/run/config/outreach.io/%s.yaml' % name,
                    subPath: '%s.yaml' % name,
                  },
                  'config-trace-volume': {
                    mountPath: '/run/config/outreach.io/trace.yaml',
                    subPath: 'trace.yaml',
                  },
                  'fflags-yaml-volume': {
                    mountPath: '/run/config/outreach.io/fflags.yaml',
                    subPath: 'fflags.yaml',
                  },

                  // default secrets
                  'secret-honeycomb-volume': {
                    mountPath: '/run/secrets/outreach.io/honeycomb',
                  },
                  'secret-launchdarkly-volume': {
                    mountPath: '/run/secrets/outreach.io/launchdarkly',
                  },

                  // user provided secrets
                  // TODO
                },
                env_+:: {
                  MY_POD_SERVICE_ACCOUNT: ok.FieldRef('spec.serviceAccountName'),
                  MY_NAMESPACE: ok.FieldRef('metadata.namespace'),
                  MY_POD_NAME: ok.FieldRef('metadata.name'),
                  MY_NODE_NAME: ok.FieldRef('spec.nodeName'),
                  MY_DEPLOYMENT: name,
                  MY_ENVIRONMENT: environment,
                  MY_CLUSTER: cluster,
                } + accountsMixin.serviceEnv,
                readinessProbe: {
                  httpGet: {
                    path: '/healthz/ready',
                    port: 'http-prom',
                  },
                  initialDelaySeconds: 5,
                  timeoutSeconds: 1,
                  periodSeconds: 15,
                },
                livenessProbe: self.readinessProbe {
                  initialDelaySeconds: 15,
                  httpGet+: {
                    path: '/healthz/live',
                  },
                },
                ports_+:: {
                  grpc: { containerPort: 5000 },
                  'http-prom': { containerPort: 8000 },
                  http: { containerPort: 8080 },
                },

                // This comes from the service's config.jsonnet
                resources: this.resources,
              },
            },
            volumes_+:: {
              // default configs
              ['config-%s' % name]: ok.ConfigMapVolume(ok.ConfigMap('config', namespace)),
              'config-trace-volume': ok.ConfigMapVolume(ok.ConfigMap('config-trace', namespace)),
              'fflags-yaml-volume': ok.ConfigMapVolume(ok.ConfigMap('fflags-yaml', namespace)),

              // default secrets
              'secret-honeycomb-volume': ok.SecretVolume(ok.Secret('honeycomb', namespace)),
              'secret-launchdarkly-volume': ok.SecretVolume(ok.Secret('launchdarkly', namespace)),

              // user provided secrets
            },
          },
        },
      },
    },
  } + if isDev then {
    // Development secrets
    honeycomb: ok.VaultSecret('honeycomb', namespace) {
      spec+: {
        path: 'dev/honeycomb/dev-env',
      },
    },
    launchdarkly: ok.VaultSecret('launchdarkly', namespace) {
      spec+: {
        path: 'deploy/launchdarkly/dev/launchdarkly',
      },
    },

  } + {
    // Append the generated secrets
    ['secret_%s' % secret.name]: ok.VaultSecret(secret.name, namespace) {
      spec+: {
        path: secret.path,
      },
    }
    for secret in vault_secrets
  } else {},
}

import { request } from './request'

export const ADMIN_API_MODE_LEGACY = 'legacy'
export const ADMIN_API_MODE_PUBLIC = 'public'

function adminBase(mode) {
  switch (mode) {
    case ADMIN_API_MODE_LEGACY:
      return ''
    case ADMIN_API_MODE_PUBLIC:
      return '/api/admin'
    default:
      throw new Error(`Unsupported admin API mode: ${mode}`)
  }
}

export function createAdminApi(mode, requester = request) {
  const base = adminBase(mode)
  const deploymentsPath = `${base}/deployments`

  function deploymentPath(app, suffix = '') {
    return `${deploymentsPath}/${encodeURIComponent(app)}${suffix}`
  }

  return {
    mode,
    supportsSession: mode === ADMIN_API_MODE_PUBLIC,

    getSession() {
      if (mode === ADMIN_API_MODE_LEGACY) {
        return Promise.resolve(null)
      }

      return requester(`${base}/session`)
    },

    getDeployments() {
      return requester(deploymentsPath)
    },

    getMiniBaseDatabases() {
      return requester(`${base}/minibase/databases`)
    },

    deployApplication({
      repoUrl,
      containerPort,
      healthPath,
      environment,
      databaseId,
    }) {
      const payload = {
        repoUrl,
      }

      if (containerPort !== undefined) {
        payload.containerPort = containerPort
      }

      if (healthPath !== undefined) {
        payload.healthPath = healthPath
      }

      if (environment !== undefined) {
        payload.environment = environment
      }

      if (databaseId !== undefined) {
        payload.databaseId = databaseId
      }

      return requester(`${base}/deploy`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(payload),
      })
    },

    getRuntimeLogs(app) {
      return requester(deploymentPath(app, '/logs'))
    },

    getDeployLogs(app) {
      return requester(deploymentPath(app, '/deploy-logs'))
    },

    getHistory(app) {
      return requester(deploymentPath(app, '/history'))
    },

    restartApplication(app) {
      return requester(deploymentPath(app, '/restart'), {
        method: 'POST',
      })
    },

    redeployApplication(app, options = {}) {
      const requestOptions = {
        method: 'POST',
      }

      if (Object.hasOwn(options, 'environment')) {
        requestOptions.headers = {
          'Content-Type': 'application/json',
        }
        requestOptions.body = JSON.stringify({
          environment: options.environment,
        })
      }

      return requester(
        deploymentPath(app, '/redeploy'),
        requestOptions,
      )
    },

    rollbackApplication(app) {
      return requester(deploymentPath(app, '/rollback'), {
        method: 'POST',
      })
    },

    deleteApplication(app) {
      return requester(deploymentPath(app), {
        method: 'DELETE',
      })
    },
  }
}

async function request(path, options = {}) {
  const response = await fetch(path, options)
  const text = await response.text()

  if (!response.ok) {
    throw new Error(text.trim() || `Request failed with HTTP ${response.status}`)
  }

  if (!text) {
    return null
  }

  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

export function getDeployments() {
  return request('/deployments')
}

export function deployApplication({
  repoUrl,
  containerPort,
  healthPath,
}) {
  return request('/deploy', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      repoUrl,
      containerPort,
      healthPath,
    }),
  })
}

export function getRuntimeLogs(app) {
  return request(
    `/deployments/${encodeURIComponent(app)}/logs`,
  )
}

export function getDeployLogs(app) {
  return request(
    `/deployments/${encodeURIComponent(app)}/deploy-logs`,
  )
}

export function getHistory(app) {
  return request(
    `/deployments/${encodeURIComponent(app)}/history`,
  )
}

export function restartApplication(app) {
  return request(
    `/deployments/${encodeURIComponent(app)}/restart`,
    { method: 'POST' },
  )
}

export function redeployApplication(app) {
  return request(
    `/deployments/${encodeURIComponent(app)}/redeploy`,
    { method: 'POST' },
  )
}

export function rollbackApplication(app) {
  return request(
    `/deployments/${encodeURIComponent(app)}/rollback`,
    { method: 'POST' },
  )
}

export function deleteApplication(app) {
  return request(
    `/deployments/${encodeURIComponent(app)}`,
    { method: 'DELETE' },
  )
}

export function safeMiniBaseDatabases(value) {
  if (!Array.isArray(value)) {
    return []
  }

  return value.flatMap((database) => {
    if (
      !database ||
      typeof database.id !== 'string' ||
      typeof database.displayName !== 'string' ||
      database.status !== 'ready' ||
      database.attached !== false
    ) {
      return []
    }

    return [{
      id: database.id,
      displayName: database.displayName,
      status: database.status,
    }]
  })
}

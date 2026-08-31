export async function request(path, options = {}) {
  const response = await fetch(path, options)
  const text = await response.text()

  if (!response.ok) {
    throw new Error(
      text.trim() || `Request failed with HTTP ${response.status}`,
    )
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

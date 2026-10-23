export function cursorUrl(path: string): string {
  const filePath = path.startsWith('/') ? path : `/${path}`
  return `cursor://file${encodeURI(filePath).replace(/[?#]/g, encodeURIComponent)}`
}

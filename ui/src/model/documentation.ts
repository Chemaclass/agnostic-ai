const WEBSITE_URL = 'https://agnostic-ai.org/'

export function documentationUrl(path: string): string {
  return new URL(path, WEBSITE_URL).href
}

/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * Build-time default for the static snapshot source, used when the URL
   * carries neither `?source=` nor `?mock=1`. Set it when the built files
   * are hosted without a server, for example on GitHub Pages.
   */
  readonly VITE_DEFAULT_SOURCE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

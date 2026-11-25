/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_WEB_API_URL?: string;
  readonly VITE_GATEWAY_URL?: string;
  readonly VITE_WS_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

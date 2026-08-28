// Build-time constants injected by `define` in vite.config.ts.

/** The console's version (ADR-0065): a TELECRAFT_VERSION environment
 * variable, else `git describe --tags --always --dirty`, else the
 * literal `development`. Read through chrome/version.ts. */
declare const __TELECRAFT_VERSION__: string

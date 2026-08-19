import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { startTheme } from './chrome/theme'
import { router } from './router'
import './fonts/fonts.css'
import './tokens.css'
import './app.css'

// The first paint is stamped by the inline resolver in `index.html`; this
// keeps the stamp correct when the operating system's scheme changes
// underneath a reader who chose `system` (ADR-0047 §2).
startTheme()

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // The fixture estate is static; surfaces share one fetch per payload.
      staleTime: 60_000,
      retry: 1,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)

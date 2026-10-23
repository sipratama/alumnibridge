import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider } from './app/auth.tsx'
import { ProtectedRoute } from './app/ProtectedRoute.tsx'
import { OnboardingPage } from './features/onboarding/OnboardingPage.tsx'
import { AccountPage } from './features/account/AccountPage.tsx'

const queryClient = new QueryClient()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}><BrowserRouter><AuthProvider><Routes>
      <Route path="/" element={<App />} />
      <Route element={<ProtectedRoute />}>
        <Route path="/onboarding" element={<OnboardingPage />} />
      </Route>
      <Route element={<ProtectedRoute requireOnboarding />}>
        <Route path="/account" element={<AccountPage />} />
        <Route path="/dashboard" element={<Navigate to="/account" replace />} />
        <Route path="/settings" element={<Navigate to="/account" replace />} />
      </Route>
      <Route path="*" element={<App />} />
    </Routes></AuthProvider></BrowserRouter></QueryClientProvider>
  </StrictMode>,
)

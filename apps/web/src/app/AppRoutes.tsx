import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import App from '@/App'
import { ProtectedRoute } from './ProtectedRoute'
import { MemberLayout } from '@/features/profile/components'
const ProfilePage = lazy(() =>
  import('@/features/profile/ProfilePage').then((module) => ({
    default: module.ProfilePage,
  })),
)
const MemberHome = lazy(() =>
  import('@/features/profile/ProfilePage').then((module) => ({
    default: module.MemberHome,
  })),
)
const PrivacyPage = lazy(() =>
  import('@/features/profile/PrivacyPage').then((module) => ({
    default: module.PrivacyPage,
  })),
)
const PublicProfilePage = lazy(() =>
  import('@/features/profile/PublicProfilePage').then((module) => ({
    default: module.PublicProfilePage,
  })),
)
const DirectoryPage = lazy(() =>
  import('@/features/directory/DirectoryPage').then((module) => ({
    default: module.DirectoryPage,
  })),
)
const OnboardingPage = lazy(() =>
  import('@/features/onboarding/OnboardingPage').then((module) => ({
    default: module.OnboardingPage,
  })),
)
const AccountPage = lazy(() =>
  import('@/features/account/AccountPage').then((module) => ({
    default: module.AccountPage,
  })),
)
export function AppRoutes() {
  return (
    <Suspense
      fallback={
        <main className="state-page" role="status">
          Loading AlumniBridge…
        </main>
      }
    >
      <Routes>
        <Route path="/" element={<App />} />
        <Route path="/alumni" element={<DirectoryPage />} />
        <Route path="/in/:slug" element={<PublicProfilePage />} />
        <Route element={<ProtectedRoute />}>
          <Route path="/onboarding" element={<OnboardingPage />} />
        </Route>
        <Route element={<ProtectedRoute requireOnboarding />}>
          <Route element={<MemberLayout />}>
            <Route path="/account" element={<AccountPage />} />
            <Route path="/dashboard" element={<MemberHome />} />
            <Route path="/profile" element={<ProfilePage />} />
            <Route path="/settings/privacy" element={<PrivacyPage />} />
            <Route
              path="/settings"
              element={<Navigate to="/settings/privacy" replace />}
            />
          </Route>
        </Route>
        <Route path="*" element={<App />} />
      </Routes>
    </Suspense>
  )
}

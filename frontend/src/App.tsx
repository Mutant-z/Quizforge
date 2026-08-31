import { useEffect } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import client from '@/api/client'

// Adaptive Layout & Page wrappers
import ResponsiveLayout from '@/components/ResponsiveLayout'
import { ResponsivePage } from '@/components/ResponsivePage'

// PC Pages
import Login from '@/pages/Login'
import Register from '@/pages/Register'
import Dashboard from '@/pages/Dashboard'
import QuestionBanks from '@/pages/QuestionBanks'
import BankDetail from '@/pages/BankDetail'
import Practice from '@/pages/Practice'
import PracticeSetup from '@/pages/PracticeSetup'
import WrongBook from '@/pages/WrongBook'
import WrongImport from '@/pages/WrongImport'
import Statistics from '@/pages/Statistics'
import AdminImports from '@/pages/admin/Imports'
import AdminCandidates from '@/pages/admin/Candidates'
import AdminQuestions from '@/pages/admin/Questions'
import Settings from '@/pages/Settings'

// Mobile Pages
import MobileLogin from '@/pages/mobile/MobileLogin'
import MobileRegister from '@/pages/mobile/MobileRegister'
import MobileDashboard from '@/pages/mobile/MobileDashboard'
import MobileQuestionBanks from '@/pages/mobile/MobileQuestionBanks'
import MobileBankDetail from '@/pages/mobile/MobileBankDetail'
import MobilePractice from '@/pages/mobile/MobilePractice'
import MobilePracticeSetup from '@/pages/mobile/MobilePracticeSetup'
import MobileWrongBook from '@/pages/mobile/MobileWrongBook'
import MobileWrongImport from '@/pages/mobile/MobileWrongImport'
import MobileStatistics from '@/pages/mobile/MobileStatistics'
import MobileAdminImports from '@/pages/mobile/MobileAdminImports'
import MobileAdminCandidates from '@/pages/mobile/MobileAdminCandidates'
import MobileAdminQuestions from '@/pages/mobile/MobileAdminQuestions'
import MobileSettings from '@/pages/mobile/MobileSettings'

function Protected({ children }: { children: React.ReactNode }) {
  const isAuthed = useAuthStore((s) => s.isAuthed)
  const location = useLocation()
  if (!isAuthed()) {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />
  }
  return <>{children}</>
}

export default function App() {
  const darkMode = useUIStore((s) => s.darkMode)
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)

  useEffect(() => {
    document.documentElement.classList.toggle('dark', darkMode)
  }, [darkMode])

  // Load current user profile on startup
  useEffect(() => {
    if (useAuthStore.getState().isAuthed() && !user) {
      client
        .get('/users/me')
        .then((r) => setUser(r.data.data))
        .catch(() => {})
    }
  }, [user, setUser])

  return (
    <Routes>
      <Route
        path="/login"
        element={<ResponsivePage desktop={<Login />} mobile={<MobileLogin />} />}
      />
      <Route
        path="/register"
        element={<ResponsivePage desktop={<Register />} mobile={<MobileRegister />} />}
      />
      <Route
        path="/"
        element={
          <Protected>
            <ResponsiveLayout />
          </Protected>
        }
      >
        <Route
          index
          element={<ResponsivePage desktop={<Dashboard />} mobile={<MobileDashboard />} />}
        />
        <Route
          path="question-banks"
          element={<ResponsivePage desktop={<QuestionBanks />} mobile={<MobileQuestionBanks />} />}
        />
        <Route
          path="question-bank/:id"
          element={<ResponsivePage desktop={<BankDetail />} mobile={<MobileBankDetail />} />}
        />
        <Route
          path="practice/setup"
          element={<ResponsivePage desktop={<PracticeSetup />} mobile={<MobilePracticeSetup />} />}
        />
        <Route
          path="practice/:sessionId"
          element={<ResponsivePage desktop={<Practice />} mobile={<MobilePractice />} />}
        />
        <Route
          path="wrong-import"
          element={<ResponsivePage desktop={<WrongImport />} mobile={<MobileWrongImport />} />}
        />
        <Route
          path="wrong-book"
          element={<ResponsivePage desktop={<WrongBook />} mobile={<MobileWrongBook />} />}
        />
        <Route
          path="statistics"
          element={<ResponsivePage desktop={<Statistics />} mobile={<MobileStatistics />} />}
        />
        <Route
          path="admin/imports"
          element={<ResponsivePage desktop={<AdminImports />} mobile={<MobileAdminImports />} />}
        />
        <Route
          path="admin/candidates"
          element={<ResponsivePage desktop={<AdminCandidates />} mobile={<MobileAdminCandidates />} />}
        />
        <Route
          path="admin/questions"
          element={<ResponsivePage desktop={<AdminQuestions />} mobile={<MobileAdminQuestions />} />}
        />
        <Route
          path="settings"
          element={<ResponsivePage desktop={<Settings />} mobile={<MobileSettings />} />}
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

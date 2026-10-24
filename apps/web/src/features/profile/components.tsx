import { useEffect, useId, useRef, type ReactNode } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'
import {
  BriefcaseBusiness,
  Check,
  GraduationCap,
  Home,
  LockKeyhole,
  LogOut,
  MapPin,
  ShieldCheck,
  UserRound,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/app/authContext'
import { errorMessage, type EducationInput, type ExperienceInput } from './api'

export function Brand() {
  return (
    <Link className="brand" to="/">
      <span className="brand-mark" aria-hidden="true">
        <span />
        <span />
        <span />
      </span>
      <span>
        Alumni<span className="brand-accent">Bridge</span>
      </span>
    </Link>
  )
}
export function MemberLayout() {
  const { logout } = useAuth()
  return (
    <div className="member-app">
      <a className="skip-link" href="#member-main">
        Skip to content
      </a>
      <header className="member-header">
        <div className="container">
          <Brand />
          <Button variant="outline" onClick={() => void logout()}>
            <LogOut size={16} aria-hidden="true" /> Sign out
          </Button>
        </div>
      </header>
      <div className="container member-layout">
        <nav className="member-nav" aria-label="Member navigation">
          {[
            ['/dashboard', 'Home', Home],
            ['/profile', 'My Profile', UserRound],
            ['/account', 'Account', GraduationCap],
            ['/settings/privacy', 'Privacy Settings', LockKeyhole],
          ].map(([to, label, Icon]) => (
            <NavLink key={String(to)} to={String(to)}>
              {typeof Icon !== 'string' && (
                <Icon size={19} aria-hidden="true" />
              )}
              <span>{String(label)}</span>
            </NavLink>
          ))}
          <p>
            Your story, on your terms.
            <br />
            You decide what to share.
          </p>
        </nav>
        <main id="member-main" className="member-main">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
export function Field({
  label,
  error,
  hint,
  children,
}: {
  label: string
  error?: string
  hint?: string
  children: (id: string, describedBy?: string) => ReactNode
}) {
  const id = useId()
  return (
    <div className="profile-field">
      <label htmlFor={id}>{label}</label>
      {children(id, error || hint ? `${id}-help` : undefined)}
      {(error || hint) && (
        <p id={`${id}-help`} className={error ? 'field-error' : 'field-hint'}>
          {error || hint}
        </p>
      )}
    </div>
  )
}
export function Feedback({
  error,
  message,
}: {
  error?: unknown
  message?: string
}) {
  return error ? (
    <p className="profile-error" role="alert">
      {errorMessage(error)}
    </p>
  ) : message ? (
    <p className="profile-success" role="status">
      <Check size={16} aria-hidden="true" />
      {message}
    </p>
  ) : null
}
export function LoadError({
  error,
  retry,
}: {
  error: unknown
  retry: () => void
}) {
  return (
    <div className="profile-panel" role="alert">
      <h2>We could not load this section.</h2>
      <p>
        {errorMessage(error).replace(
          'save your changes',
          'load this information',
        )}
      </p>
      <Button variant="outline" onClick={retry}>
        Try again
      </Button>
    </div>
  )
}
export function Loading({
  label = 'Loading your profile…',
}: {
  label?: string
}) {
  return (
    <p className="profile-loading" role="status">
      {label}
    </p>
  )
}
export function Dialog({
  title,
  children,
  onClose,
  busy = false,
}: {
  title: string
  children: ReactNode
  onClose: () => void
  busy?: boolean
}) {
  const ref = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  useEffect(() => {
    const el = ref.current
    const previousFocus = document.activeElement
    el?.showModal()
    return () => {
      el?.close()
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected)
        previousFocus.focus()
    }
  }, [])
  return (
    <dialog
      ref={ref}
      className="profile-dialog"
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault()
        if (!busy) onClose()
      }}
    >
      <div className="dialog-heading">
        <h2 id={titleId}>{title}</h2>
        <button
          className="icon-action"
          aria-label="Close dialog"
          disabled={busy}
          onClick={onClose}
        >
          <X size={21} />
        </button>
      </div>
      {children}
    </dialog>
  )
}
export function ConfirmDelete({
  title,
  busy,
  error,
  onClose,
  onConfirm,
}: {
  title: string
  busy: boolean
  error: unknown
  onClose: () => void
  onConfirm: () => void
}) {
  return (
    <Dialog title={title} onClose={onClose} busy={busy}>
      <p>
        This will remove the entry from your profile. You can add a new entry
        later.
      </p>
      <Feedback error={error} />
      <div className="form-actions">
        <Button variant="outline" disabled={busy} onClick={onClose}>
          Keep entry
        </Button>
        <Button className="danger-button" disabled={busy} onClick={onConfirm}>
          {busy ? 'Removing…' : 'Remove entry'}
        </Button>
      </div>
    </Dialog>
  )
}
export function Avatar({ name, src }: { name: string; src?: string }) {
  return src ? (
    <img className="profile-avatar" src={src} alt={`${name}'s profile`} />
  ) : (
    <span
      className="profile-avatar avatar-fallback"
      aria-label="No profile photo"
    >
      {name
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map((v) => Array.from(v)[0])
        .join('')
        .toUpperCase() || <UserRound size={34} />}
    </span>
  )
}
export function Verification({
  verified,
  pending = false,
}: {
  verified: boolean
  pending?: boolean
}) {
  return (
    <span className={verified ? 'verification verified' : 'verification'}>
      {verified && <ShieldCheck size={16} aria-hidden="true" />}
      {verified
        ? 'Verified alumni'
        : pending
          ? 'Alumni · verification pending'
          : 'Not verified as alumni'}
    </span>
  )
}
export function ExperienceList({
  items,
  actions,
}: {
  items: ExperienceInput[]
  actions?: (index: number) => ReactNode
}) {
  return (
    <ol className="timeline">
      {items.map((item, index) => (
        <li key={index}>
          <BriefcaseBusiness
            className="timeline-icon"
            size={21}
            aria-hidden="true"
          />
          <div>
            <h3>{item.job_title}</h3>
            <p className="entry-subtitle">
              {item.company}
              {item.employment_type && ` · ${item.employment_type}`}
            </p>
            <p className="entry-meta">
              {formatDate(item.start_date)} –{' '}
              {item.is_current
                ? 'Present'
                : item.end_date
                  ? formatDate(item.end_date)
                  : 'End date not specified'}
            </p>
            {item.location && (
              <p className="entry-meta">
                <MapPin size={13} aria-hidden="true" />
                {item.location}
              </p>
            )}
            {item.description && (
              <p className="prose-text">{item.description}</p>
            )}
          </div>
          {actions?.(index)}
        </li>
      ))}
    </ol>
  )
}
export function EducationList({
  items,
  actions,
}: {
  items: EducationInput[]
  actions?: (index: number) => ReactNode
}) {
  return (
    <ol className="timeline">
      {items.map((item, index) => (
        <li key={index}>
          <GraduationCap
            className="timeline-icon"
            size={22}
            aria-hidden="true"
          />
          <div>
            <h3>{item.institution}</h3>
            {(item.degree || item.field_of_study) && (
              <p className="entry-subtitle">
                {[item.degree, item.field_of_study].filter(Boolean).join(' · ')}
              </p>
            )}
            {(item.start_date || item.end_date) && (
              <p className="entry-meta">
                {item.start_date
                  ? formatDate(item.start_date)
                  : 'Start not specified'}{' '}
                –{' '}
                {item.end_date
                  ? formatDate(item.end_date)
                  : 'End not specified'}
              </p>
            )}
            {item.description && (
              <p className="prose-text">{item.description}</p>
            )}
          </div>
          {actions?.(index)}
        </li>
      ))}
    </ol>
  )
}
function formatDate(value: string) {
  return new Date(`${value}T00:00:00Z`).toLocaleDateString('en', {
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  })
}

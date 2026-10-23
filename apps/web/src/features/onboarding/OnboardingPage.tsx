import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/app/auth'
import { ApiError, request, type CurrentUser, type OnboardingInput } from '@/shared/api/client'

const schema = z.object({
  display_name: z.string().trim().min(2, 'Enter at least two characters').max(100),
  role: z.enum(['student', 'alumni']),
  graduation_year: z.string().optional(),
}).superRefine((value, context) => {
  if (value.role === 'alumni') {
    const year = Number(value.graduation_year)
    const max = new Date().getUTCFullYear() + 1
    if (!value.graduation_year || !Number.isInteger(year) || year < 1950 || year > max) {
      context.addIssue({ code: 'custom', path: ['graduation_year'], message: `Enter a year from 1950 to ${max}` })
    }
  }
})
type FormData = z.infer<typeof schema>

export function OnboardingPage() {
  const { user, getToken, refreshUser, logout } = useAuth()
  const navigate = useNavigate()
  const [serverError, setServerError] = useState('')
  const { register, handleSubmit, watch, formState: { errors, isSubmitting } } = useForm<FormData>({ resolver: zodResolver(schema), defaultValues: { role: 'student', display_name: user?.display_name ?? '', graduation_year: '' } })
  const role = watch('role')

  async function submit(values: FormData) {
    setServerError('')
    const input: OnboardingInput = { display_name: values.display_name.trim(), role: values.role, institution: 'uhamka', study_program: 'mathematics-education' }
    if (values.role === 'alumni') input.graduation_year = Number(values.graduation_year)
    try {
      await request<CurrentUser>('/auth/onboarding', getToken, { method: 'POST', body: JSON.stringify(input) })
      await refreshUser()
      navigate('/account')
    } catch (error) {
      setServerError(error instanceof ApiError && error.status === 409 ? 'A membership with different details already exists.' : 'Onboarding failed. Please check your details and try again.')
    }
  }

  return <main className="onboarding-page"><div className="onboarding-card"><div className="onboarding-top"><Link className="brand" to="/"><span className="brand-mark" aria-hidden="true"><span /><span /><span /></span><span>Alumni<span className="brand-accent">Bridge</span></span></Link><button type="button" className="plain-action" onClick={() => void logout()}>Sign out</button></div><span className="section-kicker">WELCOME TO THE COMMUNITY</span><h1>Tell us where your journey began.</h1><p className="form-intro">Join the Mathematics Education community at UHAMKA. Alumni membership is reviewed before public listing.</p><form onSubmit={handleSubmit(submit)} noValidate><fieldset className="role-options"><legend>I am joining as</legend><label><input type="radio" value="student" {...register('role')} /><span><strong>Student</strong><small>Learning and exploring what comes next</small></span></label><label><input type="radio" value="alumni" {...register('role')} /><span><strong>Alumni</strong><small>Sharing experience and staying connected</small></span></label></fieldset><label className="field-label" htmlFor="display-name">Display name</label><input id="display-name" className="text-input" autoComplete="name" {...register('display_name')} aria-invalid={!!errors.display_name} /><p className="field-error" role="alert">{errors.display_name?.message}</p><div className="affiliation"><span>University</span><strong>UHAMKA</strong><span>Study program</span><strong>Mathematics Education</strong></div>{role === 'alumni' && <><label className="field-label" htmlFor="graduation-year">Graduation year</label><input id="graduation-year" className="text-input" type="number" min="1950" max={new Date().getUTCFullYear() + 1} {...register('graduation_year')} aria-invalid={!!errors.graduation_year} /><p className="field-error" role="alert">{errors.graduation_year?.message}</p></>}{serverError && <p className="form-server-error" role="alert">{serverError}</p>}<Button type="submit" size="lg" disabled={isSubmitting} className="submit-button">{isSubmitting ? 'Saving…' : 'Complete onboarding'}</Button></form></div></main>
}

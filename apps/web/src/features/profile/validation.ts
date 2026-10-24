import { z } from 'zod'

const text = (max: number, min = 0) =>
  z
    .string()
    .transform((v) => v.trim())
    .refine(
      (v) => Array.from(v).length >= min,
      `Enter at least ${min} characters.`,
    )
    .refine(
      (v) => Array.from(v).length <= max,
      `Use ${max} characters or fewer.`,
    )
    .refine((v) => !v.includes('\0'), 'Remove invalid characters.')
const compact = (max: number, min = 0) =>
  z
    .string()
    .transform((v) => v.trim().replace(/\s+/gu, ' '))
    .pipe(text(max, min))
const reserved = new Set([
  'api',
  'admin',
  'auth',
  'in',
  'me',
  'settings',
  'dashboard',
  'login',
  'logout',
  'register',
  'onboarding',
  'profile',
  'profiles',
])
export const profileSchema = z.object({
  display_name: compact(100, 2),
  headline: compact(160),
  bio: text(5000),
  location: compact(120),
  current_company: compact(160),
  current_job_title: compact(160),
  slug: text(64, 3).refine(
    (v) => /^[a-z0-9]+(-[a-z0-9]+)*$/.test(v) && !reserved.has(v),
    'Use lowercase letters, numbers and single internal hyphens; choose a non-reserved address.',
  ),
})
const date = z
  .string()
  .refine(
    (v) =>
      !v ||
      (/^\d{4}-\d{2}-\d{2}$/.test(v) &&
        !v.startsWith('0000') &&
        !Number.isNaN(Date.parse(`${v}T00:00:00Z`)) &&
        new Date(`${v}T00:00:00Z`).toISOString().slice(0, 10) === v),
    'Enter a valid calendar date.',
  )
export const experienceSchema = z
  .object({
    company: compact(160, 1),
    job_title: compact(160, 1),
    employment_type: compact(80),
    location: compact(120),
    start_date: date.refine(Boolean, 'Choose a start date.'),
    end_date: date,
    is_current: z.boolean(),
    description: text(5000),
  })
  .superRefine((v, ctx) => {
    if (!v.is_current && v.end_date && v.start_date > v.end_date)
      ctx.addIssue({
        code: 'custom',
        path: ['end_date'],
        message: 'End date must be on or after the start date.',
      })
  })
export const educationSchema = z
  .object({
    institution: compact(200, 1),
    degree: compact(160),
    field_of_study: compact(160),
    start_date: date,
    end_date: date,
    description: text(5000),
  })
  .superRefine((v, ctx) => {
    if (v.start_date && v.end_date && v.start_date > v.end_date)
      ctx.addIssue({
        code: 'custom',
        path: ['end_date'],
        message: 'End date must be on or after the start date.',
      })
  })
export function validatePhoto(file: File): string | null {
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type))
    return 'Choose a JPEG, PNG or WebP image.'
  if (file.size > 5 * 1024 * 1024) return 'Choose an image smaller than 5 MiB.'
  if (!file.size) return 'This file is empty. Choose another image.'
  return null
}

import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import PublicHome from './PublicHome'

describe('public landing page', () => {
  test('explains exact supported strategies and both intended entry paths', () => {
    render(<PublicHome />)

    expect(
      screen.getByRole('heading', {
        name: /Run supported application repositories on your own server/i,
      }),
    ).toBeTruthy()

    expect(
      screen.getByRole('link', { name: /Guest Overview/i }).href,
    ).toContain('/guest/')

    expect(
      screen.getByRole('link', { name: /Open Administrator/i }).href,
    ).toContain('/admin/')
    expect(screen.getByText('What it does')).toBeTruthy()
    expect(screen.getByText('How it works')).toBeTruthy()
    expect(screen.getByText('Why it is useful')).toBeTruthy()
    expect(
      screen.getByRole('heading', { name: 'Currently supported' }),
    ).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Dockerfile' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Vite' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Node + Express' })).toBeTruthy()
    expect(
      screen.getByRole('heading', { name: 'Vite + Node/Express' }),
    ).toBeTruthy()
    expect(screen.getByText(/Dockerfile is the escape hatch/)).toBeTruthy()
    expect(
      screen.getByText(/MiniBase attachment currently targets the Node\/Express backend paths/),
    ).toBeTruthy()
    expect(
      screen.getByText(/Administrator access opens deployment and lifecycle controls/),
    ).toBeTruthy()
    expect(
      screen.queryByText(/Administrator access is protected by Cloudflare Access/),
    ).toBeNull()
  })
})

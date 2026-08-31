import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import PublicHome from './PublicHome'

describe('public landing page', () => {
  test('explains MiniDeploy and offers both intended entry paths', () => {
    render(<PublicHome />)

    expect(
      screen.getByRole('heading', {
        name: /Ship software on.*infrastructure you own/i,
      }),
    ).toBeTruthy()

    expect(
      screen.getByRole('link', { name: /Continue as Guest/i }).href,
    ).toContain('/guest/')

    const adminLinks = screen.getAllByRole('link', {
      name: /Admin Sign In/i,
    })

    expect(adminLinks.every((link) => link.href.includes('/admin/'))).toBe(
      true,
    )
  })
})

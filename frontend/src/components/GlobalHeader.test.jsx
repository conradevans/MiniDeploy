import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import GlobalHeader from './GlobalHeader'

afterEach(cleanup)

function productNames(container) {
  const navigation = within(container).getByRole('navigation', {
    name: 'ReactorLab products',
  })
  return within(navigation).getAllByRole('link').map((link) => link.textContent)
}

describe('MiniDeploy global header', () => {
  test.each([
    ['root', '', ['ReactorLab', 'MiniDeploy', 'MiniBase']],
    ['guest', 'Guest View', ['ReactorLab', 'MiniDeploy', 'MiniBase']],
    ['admin', 'Admin · user@example.com', ['ReactorLab', 'MiniDeploy', 'MiniBase', 'MiniAI']],
  ])('shows the correct %s controls', (mode, sessionLabel, products) => {
    const view = render(
      <GlobalHeader mode={mode} sessionLabel={sessionLabel} />,
    )
    expect(productNames(view.container)).toEqual(products)
    expect(
      screen.getByRole('link', { name: 'MiniDeploy' }).getAttribute('aria-current'),
    ).toBe('page')

    if (mode === 'admin') {
      expect(screen.getByText('ACCESS SESSION').tagName).toBe('SMALL')
      expect(screen.getByText('user@example.com').tagName).toBe('STRONG')
      expect(
        screen.getByLabelText('Access session: user@example.com'),
      ).toBeTruthy()
    } else if (sessionLabel) {
      expect(screen.getByText(sessionLabel).tagName).toBe('DIV')
    }

    if (sessionLabel) {
      expect(screen.getByRole('link', { name: 'Switch Access' })).toBeTruthy()
    } else {
      expect(screen.queryByRole('link', { name: 'Switch Access' })).toBeNull()
    }
  })

  test('uses the safe administrator fallback without inventing an email', () => {
    render(<GlobalHeader mode="admin" sessionLabel="Admin" />)
    expect(screen.getByText('Administrator')).toBeTruthy()
  })

  test('mobile menu closes on selection, outside press, and Escape', () => {
    render(<GlobalHeader mode="guest" sessionLabel="Guest View" />)
    const open = screen.getByRole('button', { name: 'Open product menu' })

    fireEvent.click(open)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('button', { name: 'Close product menu' })).toBeNull()

    fireEvent.click(open)
    fireEvent.pointerDown(document.body)
    expect(screen.queryByRole('button', { name: 'Close product menu' })).toBeNull()

    fireEvent.click(open)
    const mobileNav = screen.getAllByRole('navigation', {
      name: 'ReactorLab products',
    })[1]
    const destination = within(mobileNav).getByRole('link', {
      name: 'ReactorLab',
    })
    destination.addEventListener('click', (event) => event.preventDefault(), {
      once: true,
    })
    fireEvent.click(destination)
    expect(screen.queryByRole('button', { name: 'Close product menu' })).toBeNull()
  })
})

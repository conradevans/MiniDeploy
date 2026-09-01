import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import DatabaseDialog from './DatabaseDialog'
import Modal from './Modal'

afterEach(cleanup)

const deployment = { app: 'my-scheduler', strategy: 'node-express' }

function api(overrides = {}) {
  return {
    getMiniBaseDatabases: vi.fn().mockResolvedValue([]),
    attachMiniBaseDatabase: vi.fn().mockResolvedValue({}),
    ...overrides,
  }
}

function renderDialog(client, overrides = {}) {
  const onClose = overrides.onClose || vi.fn()
  const onAttached = overrides.onAttached || vi.fn().mockResolvedValue(undefined)
  return {
    onClose,
    onAttached,
    ...render(
      <Modal title="Add MiniBase Database · my-scheduler" onClose={onClose}>
        <DatabaseDialog api={client} deployment={deployment} onClose={onClose} onAttached={onAttached} />
      </Modal>,
    ),
  }
}

describe('DatabaseDialog', () => {
  test('creates a database with a sensible default and prevents duplicate pending submits', async () => {
    let resolveAttach
    const client = api({
      attachMiniBaseDatabase: vi.fn(() => new Promise((resolve) => { resolveAttach = resolve })),
    })
    const onAttached = vi.fn().mockResolvedValue(undefined)
    renderDialog(client, { onAttached })

    expect(screen.getByRole('dialog')).toBeTruthy()
    expect(screen.getByLabelText('Display name').value).toBe('My Scheduler Production')
    const submit = screen.getByRole('button', { name: 'Create and attach' })
    fireEvent.click(submit)
    fireEvent.click(submit)
    expect(client.attachMiniBaseDatabase).toHaveBeenCalledTimes(1)
    expect(client.attachMiniBaseDatabase).toHaveBeenCalledWith('my-scheduler', {
      mode: 'create',
      displayName: 'My Scheduler Production',
    })
    expect(screen.getByRole('button', { name: 'Attaching database…' }).disabled).toBe(true)

    await act(async () => { resolveAttach({}) })
    await waitFor(() => expect(onAttached).toHaveBeenCalledTimes(1))
  })

  test('attaches only ready unattached allowlisted databases without rendering injected credentials', async () => {
    const client = api({
      getMiniBaseDatabases: vi.fn().mockResolvedValue([
        {
          id: 'database_0123456789abcdef0123456789abcdef',
          displayName: 'Existing Production',
          status: 'ready',
          attached: false,
          password: 'mock-password-must-not-render',
          databaseUrl: 'postgresql://must-not-render',
        },
        {
          id: 'database_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
          displayName: 'Already attached',
          status: 'ready',
          attached: true,
        },
      ]),
    })
    const view = renderDialog(client)
    fireEvent.click(screen.getByLabelText('Attach existing'))
    const select = await screen.findByLabelText('Ready database')
    expect(within(select).getAllByRole('option')).toHaveLength(1)
    expect(screen.getByText('Existing Production · Ready')).toBeTruthy()
    expect(view.container.textContent).not.toContain('mock-password-must-not-render')
    expect(view.container.textContent).not.toContain('postgresql://')
    fireEvent.click(screen.getByRole('button', { name: 'Attach database' }))
    await waitFor(() => expect(client.attachMiniBaseDatabase).toHaveBeenCalledWith('my-scheduler', {
      mode: 'attach',
      databaseId: 'database_0123456789abcdef0123456789abcdef',
    }))
  })

  test('shows empty and unavailable MiniBase states safely', async () => {
    const empty = renderDialog(api())
    fireEvent.click(screen.getByLabelText('Attach existing'))
    expect(await screen.findByText('No ready unattached MiniBase databases are available.')).toBeTruthy()
    empty.unmount()

    const failed = renderDialog(api({
      getMiniBaseDatabases: vi.fn().mockRejectedValue(new Error('postgresql://secret@host/database')),
    }))
    expect(await screen.findByText('MiniBase is unavailable.')).toBeTruthy()
    expect(failed.container.textContent).not.toContain('postgresql://')
  })

  test('reports a safe attach failure and supports Escape dismissal', async () => {
    const client = api({
      attachMiniBaseDatabase: vi.fn().mockRejectedValue(new Error('password=must-not-render')),
    })
    const onClose = vi.fn()
    const view = renderDialog(client, { onClose })
    fireEvent.click(screen.getByRole('button', { name: 'Create and attach' }))
    expect(await screen.findByText('MiniBase could not attach the database. The existing deployment remains available.')).toBeTruthy()
    expect(view.container.textContent).not.toContain('must-not-render')
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

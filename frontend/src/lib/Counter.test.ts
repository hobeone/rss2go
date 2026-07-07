import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect } from 'vitest'
import Counter from './Counter.svelte'

describe('Counter', () => {
  it('renders with initial count 0', () => {
    render(Counter)
    const button = screen.getByRole('button')
    expect(button).toHaveTextContent('Count is 0')
  })

  it('increments count on click', async () => {
    render(Counter)
    const button = screen.getByRole('button')
    await fireEvent.click(button)
    expect(button).toHaveTextContent('Count is 1')
  })
})

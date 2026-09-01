import { describe, expect, it } from 'vitest'
import { computeLayeredLayout } from './layout'

describe('computeLayeredLayout', () => {
  it('places a simple chain left-to-right in increasing layers', () => {
    const nodes = ['gateway', 'order', 'payment']
    const edges = [
      { source: 'gateway', target: 'order' },
      { source: 'order', target: 'payment' },
    ]

    const { positions, layers } = computeLayeredLayout(nodes, edges)

    expect(layers).toEqual([['gateway'], ['order'], ['payment']])
    expect(positions.get('gateway')!.x).toBeLessThan(positions.get('order')!.x)
    expect(positions.get('order')!.x).toBeLessThan(positions.get('payment')!.x)
  })

  it('is deterministic: identical input produces identical output', () => {
    const nodes = ['a', 'b', 'c', 'd']
    const edges = [
      { source: 'a', target: 'b' },
      { source: 'a', target: 'c' },
      { source: 'b', target: 'd' },
      { source: 'c', target: 'd' },
    ]

    const first = computeLayeredLayout(nodes, edges)
    const second = computeLayeredLayout(nodes, edges)

    expect(first.layers).toEqual(second.layers)
    expect([...first.positions.entries()]).toEqual([...second.positions.entries()])
  })

  it('breaks ties within a layer alphabetically', () => {
    const nodes = ['gateway', 'inventory', 'payment']
    const edges = [
      { source: 'gateway', target: 'payment' },
      { source: 'gateway', target: 'inventory' },
    ]

    const { layers } = computeLayeredLayout(nodes, edges)

    expect(layers[1]).toEqual(['inventory', 'payment'])
  })

  it('terminates and assigns every node a position even with a cycle', () => {
    const nodes = ['a', 'b', 'c']
    const edges = [
      { source: 'a', target: 'b' },
      { source: 'b', target: 'c' },
      { source: 'c', target: 'a' },
    ]

    const { positions, layers } = computeLayeredLayout(nodes, edges)

    expect(positions.size).toBe(3)
    expect(layers.flat().length).toBe(3)
  })

  it('handles an empty graph', () => {
    const { positions, layers } = computeLayeredLayout([], [])
    expect(positions.size).toBe(0)
    expect(layers).toEqual([])
  })
})

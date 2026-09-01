import { Slot } from '@radix-ui/react-slot'
import clsx from 'clsx'
import type { ButtonHTMLAttributes } from 'react'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean
  variant?: 'primary' | 'secondary' | 'ghost'
  size?: 'sm' | 'md'
}

const VARIANT_CLASSES: Record<NonNullable<ButtonProps['variant']>, string> = {
  primary: 'bg-accent text-white hover:bg-accent/90',
  secondary:
    'bg-surface-2 text-text-primary border border-border-default hover:bg-surface-3',
  ghost: 'text-text-secondary hover:bg-surface-2 hover:text-text-primary',
}

const SIZE_CLASSES: Record<NonNullable<ButtonProps['size']>, string> = {
  sm: 'h-7 px-2.5 text-xs',
  md: 'h-8 px-3 text-sm',
}

export function Button({
  asChild,
  variant = 'secondary',
  size = 'md',
  className,
  disabled,
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot : 'button'
  return (
    <Comp
      className={clsx(
        'inline-flex items-center justify-center gap-1.5 rounded-sm font-medium transition-colors',
        'disabled:cursor-not-allowed disabled:opacity-40',
        VARIANT_CLASSES[variant],
        SIZE_CLASSES[size],
        className,
      )}
      disabled={disabled}
      {...props}
    />
  )
}

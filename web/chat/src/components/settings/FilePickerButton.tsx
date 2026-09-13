import { useRef, type ChangeEvent } from 'react'
import { Button } from '../ui'

export interface FilePickerButtonProps {
  accept: string
  chooseLabel: string
  clearLabel?: string
  fileName?: string | null
  disabled?: boolean
  onFile: (file: File | undefined) => void
  onClear?: () => void
  'aria-label'?: string
}

export function FilePickerButton(props: FilePickerButtonProps) {
  const ref = useRef<HTMLInputElement>(null)
  const onChange = (e: ChangeEvent<HTMLInputElement>) => {
    props.onFile(e.target.files?.[0])
    e.target.value = ''
  }
  return (
    <div className="file-picker">
      <input
        ref={ref}
        type="file"
        accept={props.accept}
        hidden
        disabled={props.disabled}
        aria-label={props['aria-label'] ?? props.chooseLabel}
        onChange={onChange}
      />
      <Button
        type="button"
        variant="secondary"
        size="sm"
        disabled={props.disabled}
        onClick={() => ref.current?.click()}
      >
        {props.chooseLabel}
      </Button>
      {props.fileName ? (
        <span className="file-picker-name">
          {props.fileName}
          {props.onClear && props.clearLabel ? (
            <Button type="button" variant="ghost" size="sm" disabled={props.disabled} onClick={props.onClear}>
              {props.clearLabel}
            </Button>
          ) : null}
        </span>
      ) : null}
    </div>
  )
}

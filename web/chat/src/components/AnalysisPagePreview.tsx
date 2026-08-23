export interface AnalysisPagePreviewProps {
  artifactUrl: string
}

export function AnalysisPagePreview({ artifactUrl }: AnalysisPagePreviewProps) {
  return (
    <div className="analysis-page-preview">
      <iframe
        sandbox="allow-scripts"
        src={artifactUrl}
        height={480}
        title="分析页预览"
      />
      <a
        className="analysis-page-preview-link"
        href={artifactUrl}
        target="_blank"
        rel="noopener noreferrer"
      >
        在新标签页打开
      </a>
    </div>
  )
}

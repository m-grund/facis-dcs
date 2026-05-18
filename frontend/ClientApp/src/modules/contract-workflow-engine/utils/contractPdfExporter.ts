import pdfMake from 'pdfmake/build/pdfmake'
import pdfFonts from 'pdfmake/build/vfs_fonts'
import type { ContentText, TDocumentDefinitions } from 'pdfmake/interfaces'
import type { PdfDataResult } from './contractPdfConverter'

type PdfMakeWithVfs = typeof pdfMake & {
  /** virtual file system */
  vfs?: Record<string, string>
}

interface ExportOptions {
  displayTitleInContent?: boolean
}

let isPdfMakeConfigured = false

function configurePdfMake(): void {
  if (isPdfMakeConfigured) return

  const pdfMakeInstance = pdfMake as PdfMakeWithVfs
  const fontFileSystem = pdfFonts as unknown as { vfs?: Record<string, string> }
  if (fontFileSystem.vfs) {
    // Register the bundled virtual font filesystem (Roboto, etc.) with pdfMake
    // so createPdf() can resolve fonts in the browser build.
    pdfMakeInstance.vfs = fontFileSystem.vfs
  }

  isPdfMakeConfigured = true
}

function toPdfDocumentDefinition(pdfData: PdfDataResult, title = 'Contract Document', options?: ExportOptions): TDocumentDefinitions {
  const displayTitleInContent = options?.displayTitleInContent ?? false
  const content = displayTitleInContent ? [getTitleNode(title), ...pdfData.content] : pdfData.content
  return {
    content: content,
    styles: pdfData.styles,
    info: {
      title
    },
    version: pdfData.version,
    subset: pdfData.subset,
    tagged: pdfData.tagged,
    displayTitle: pdfData.displayTitle
  }
}

export function downloadContractPdf(pdfData: PdfDataResult, filename = 'contract.pdf', title?: string, options?: ExportOptions): void {
  configurePdfMake()

  const documentDefinition = toPdfDocumentDefinition(pdfData, title, options)
  pdfMake.createPdf(documentDefinition).download(filename)
}


function getTitleNode(title: string): ContentText {
  return {
    text: title,
    style: 'docTitle',
    alignment: 'center',
    margin: [0, 0, 0, 12]
  }
}
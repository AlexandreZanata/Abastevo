/**
 * Tutorial lesson data contract (LR13).
 * Pure validation for `landing/static/como-usar/<slug>/lesson.json`.
 * Chapters render as static HTML with inline SVG overlays; no DOM code lives here,
 * so instructions stay readable and searchable without JavaScript.
 */

export interface LessonTarget {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface LessonStep {
  stepId: string;
  image: string;
  imageSha256: string;
  nativeWidth: number;
  nativeHeight: number;
  state: string;
  prerequisite: string;
  instructionPtBr: string;
  controlLabelPtBr: string;
  expectedPtBr: string;
  recoveryPtBr: string;
  altPtBr: string;
  target?: LessonTarget;
  availability: string;
  claimClass: string;
  sourceFiles: string[];
  capturedAt: string;
  privacyReviewed: boolean;
  demoOnly: boolean;
}

export interface Lesson {
  lessonId: string;
  slug: string;
  titlePtBr: string;
  appVersion: string;
  appSourceSha: string | null;
  steps: LessonStep[];
}

const AVAILABILITY = new Set(['Disponível', 'Em breve', 'Planejado']);
const HEX64 = /^[a-f0-9]{64}$/;

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0;
}

/**
 * Validate lesson data. Returns a list of human-readable errors; empty means valid.
 */
export function validateLesson(data: unknown): string[] {
  const errors: string[] = [];
  if (typeof data !== 'object' || data === null) {
    return ['lesson must be an object'];
  }
  const lesson = data as Record<string, unknown>;
  for (const field of ['lessonId', 'slug', 'titlePtBr', 'appVersion']) {
    if (!isNonEmptyString(lesson[field])) {
      errors.push(`missing or empty ${field}`);
    }
  }
  if (lesson['appSourceSha'] !== null && !isNonEmptyString(lesson['appSourceSha'])) {
    errors.push('appSourceSha must be null or a non-empty string');
  }
  if (!Array.isArray(lesson['steps']) || lesson['steps'].length === 0) {
    errors.push('steps must be a non-empty array');
    return errors;
  }
  const seen = new Set<string>();
  lesson['steps'].forEach((rawStep: unknown, index: number) => {
    const where = `steps[${index}]`;
    if (typeof rawStep !== 'object' || rawStep === null) {
      errors.push(`${where} must be an object`);
      return;
    }
    const step = rawStep as Record<string, unknown>;
    if (!isNonEmptyString(step['stepId'])) {
      errors.push(`${where} missing stepId`);
    } else if (seen.has(step['stepId'] as string)) {
      errors.push(`${where} duplicate stepId ${step['stepId']}`);
    } else {
      seen.add(step['stepId'] as string);
    }
    for (const field of [
      'image', 'state', 'prerequisite', 'instructionPtBr', 'controlLabelPtBr',
      'expectedPtBr', 'recoveryPtBr', 'altPtBr', 'availability', 'claimClass', 'capturedAt',
    ]) {
      if (!isNonEmptyString(step[field])) {
        errors.push(`${where} missing or empty ${field}`);
      }
    }
    if (typeof step['imageSha256'] !== 'string' || !HEX64.test(step['imageSha256'])) {
      errors.push(`${where} imageSha256 must be lowercase hex64`);
    }
    if (!Number.isInteger(step['nativeWidth']) || (step['nativeWidth'] as number) <= 0 ||
        !Number.isInteger(step['nativeHeight']) || (step['nativeHeight'] as number) <= 0) {
      errors.push(`${where} native dimensions must be positive integers`);
    }
    if (typeof step['availability'] === 'string' && !AVAILABILITY.has(step['availability'])) {
      errors.push(`${where} unknown availability ${step['availability']}`);
    }
    if (!Array.isArray(step['sourceFiles'])) {
      errors.push(`${where} sourceFiles must be an array`);
    }
    if (typeof step['privacyReviewed'] !== 'boolean' || typeof step['demoOnly'] !== 'boolean') {
      errors.push(`${where} privacyReviewed and demoOnly must be booleans`);
    }
    if (step['target'] !== undefined) {
      const target = step['target'] as Record<string, unknown>;
      const dimsOk = typeof step['nativeWidth'] === 'number' && typeof step['nativeHeight'] === 'number';
      for (const field of ['x', 'y', 'width', 'height']) {
        if (!Number.isFinite(target[field]) || (target[field] as number) < 0) {
          errors.push(`${where}.target.${field} must be a non-negative number`);
        }
      }
      if (dimsOk && (target['width'] as number) > 0 && (target['height'] as number) > 0) {
        const w = step['nativeWidth'] as number;
        const h = step['nativeHeight'] as number;
        if ((target['x'] as number) + (target['width'] as number) > w ||
            (target['y'] as number) + (target['height'] as number) > h) {
          errors.push(`${where}.target exceeds ${w}x${h}`);
        }
      } else if (dimsOk) {
        errors.push(`${where}.target width/height must be positive`);
      }
    }
  });
  return errors;
}

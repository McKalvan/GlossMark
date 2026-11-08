import React from 'react';
import Chip from '@material-ui/core/Chip';
import Tooltip from '@material-ui/core/Tooltip';
import { StatusError, StatusOK, StatusPending, StatusWarning } from '@backstage/core-components';
import { Criticality, Severity } from '../types';

/** A criticality level as a small chip; nothing when unset. */
export function CriticalityBadge(props: { level?: Criticality; from?: string }) {
  const { level, from } = props;
  if (!level) return null;
  const chip = (
    <Chip
      size="small"
      label={level}
      color={level === 'critical' || level === 'high' ? 'secondary' : 'default'}
      variant={level === 'low' ? 'outlined' : 'default'}
      style={{ marginRight: 4 }}
    />
  );
  return from && from !== 'declared' ? <Tooltip title={`inherited from ${from}`}>{chip}</Tooltip> : chip;
}

/** A finding's severity, or a clean mark, with Backstage's status icons. */
export function SeverityStatus(props: { severity?: Severity; children?: React.ReactNode }) {
  const { severity, children } = props;
  switch (severity) {
    case 'error':
      return <StatusError>{children}</StatusError>;
    case 'warning':
      return <StatusWarning>{children}</StatusWarning>;
    case 'info':
      return <StatusPending>{children}</StatusPending>;
    default:
      return <StatusOK>{children}</StatusOK>;
  }
}

/** The gate class a decision had (AUTO, SOFT, HARD). */
export function GateBadge(props: { gate?: string }) {
  if (!props.gate) return null;
  return <Chip size="small" variant="outlined" label={props.gate} />;
}

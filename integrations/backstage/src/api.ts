import {
  ConfigApi,
  createApiRef,
  FetchApi,
} from '@backstage/core-plugin-api';
import { checkBoundaryDocument, checkLandscapeDocument, slugOf } from './mapping';
import { BoundaryDocument, LandscapeDocument } from './types';

/** Reads the documents gloss export --backstage wrote. */
export interface GlossMarkApi {
  /** The base URL the documents are served from (glossmark.baseUrl), or undefined. */
  baseUrl(): string | undefined;
  /** The URL of a boundary's document, from its CURIE or slug and the base URL. */
  documentUrl(boundary: string): string | undefined;
  getBoundary(url: string): Promise<BoundaryDocument>;
  getLandscape(): Promise<LandscapeDocument>;
}

export const glossMarkApiRef = createApiRef<GlossMarkApi>({ id: 'plugin.glossmark.api' });

export class GlossMarkClient implements GlossMarkApi {
  private readonly configApi: ConfigApi;
  private readonly fetchApi: FetchApi;

  constructor(options: { configApi: ConfigApi; fetchApi: FetchApi }) {
    this.configApi = options.configApi;
    this.fetchApi = options.fetchApi;
  }

  baseUrl(): string | undefined {
    const u = this.configApi.getOptionalString('glossmark.baseUrl');
    return u ? u.replace(/\/+$/, '') : undefined;
  }

  documentUrl(boundary: string): string | undefined {
    const base = this.baseUrl();
    return base ? `${base}/${slugOf(boundary)}.json` : undefined;
  }

  async getBoundary(url: string): Promise<BoundaryDocument> {
    return checkBoundaryDocument(await this.getJson(url));
  }

  async getLandscape(): Promise<LandscapeDocument> {
    const base = this.baseUrl();
    if (!base) {
      throw new Error('glossmark.baseUrl is not set in app-config.yaml: point it at the directory gloss export --backstage wrote');
    }
    return checkLandscapeDocument(await this.getJson(`${base}/landscape.json`));
  }

  private async getJson(url: string): Promise<unknown> {
    const res = await this.fetchApi.fetch(url, { headers: { Accept: 'application/json' } });
    if (!res.ok) {
      throw new Error(`GET ${url}: ${res.status} ${res.statusText}`);
    }
    return res.json();
  }
}

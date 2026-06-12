export class ApiError extends Error {
  status: number
  title: string
  detail: string

  constructor(detail: string, status: number, title = "") {
    super(detail)
    this.name = "ApiError"
    this.status = status
    this.title = title
    this.detail = detail
  }
}

export async function apiFetch<T>(url: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...opts,
    headers: {
      Accept: "application/json",
      ...opts?.headers,
    },
  })

  const contentType = res.headers.get("Content-Type") || ""

  if (contentType.includes("application/problem+json")) {
    const problem = await res.json()
    throw new ApiError(problem.detail, problem.status, problem.title)
  }

  if (!res.ok) {
    throw new ApiError(res.statusText || "request failed", res.status)
  }

  const json = await res.json()
  return json.data ?? json
}

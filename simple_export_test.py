#!/usr/bin/env python3

"""
Simple Export Test - One export with status polling
Usage: python simple_export_test.py
"""

import argparse
import json
import os
import sys
import time
from datetime import datetime, timedelta
from pathlib import Path

import requests


def get_request_id(response):
    """Extract request ID from response headers"""
    for header in ['x-rh-insights-request-id', 'x-rh-request-id', 'x-request-id']:
        request_id = response.headers.get(header)
        if request_id:
            return request_id
    return None


def main():
    """Simple export test workflow"""
    parser = argparse.ArgumentParser(description="Simple Export Service Test")
    parser.add_argument(
        '--url',
        default=os.environ.get('PUBLIC_URL', 'http://localhost:8000/api/export/v1'),
        help='Export service API URL'
    )
    parser.add_argument(
        '--auth-token',
        default=os.environ.get('AUTH_TOKEN') or os.environ.get('RH_IDENTITY', 'eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiJhY2NvdW50MTIzIiwib3JnX2lkIjoib3JnMTIzIiwidHlwZSI6IlVzZXIiLCJ1c2VyIjp7ImlzX29yZ19hZG1pbiI6dHJ1ZX0sImludGVybmFsIjp7Im9yZ19pZCI6Im9yZzEyMyJ9fX0K'),
        help='Authentication token - either x-rh-identity value or JWT token'
    )
    parser.add_argument(
        '--auth-type',
        choices=['identity', 'jwt'],
        default=os.environ.get('AUTH_TYPE', 'identity'),
        help='Authentication type: "identity" for x-rh-identity header, "jwt" for Authorization: Bearer (default: identity)'
    )
    parser.add_argument(
        '--output',
        help='Output file path (default: <export_id>.zip)'
    )
    parser.add_argument(
        '--max-attempts',
        type=int,
        default=60,
        help='Maximum status check attempts (default: 60)'
    )
    parser.add_argument(
        '--poll-interval',
        type=int,
        default=2,
        help='Seconds between status checks (default: 2)'
    )

    parser.add_argument(
        '--proxy',
        default=os.environ.get('HTTP_PROXY') or os.environ.get('HTTPS_PROXY'),
        help='HTTP/HTTPS proxy URL (e.g., http://proxy.example.com:3128) (default: $HTTP_PROXY or $HTTPS_PROXY)'
    )

    args = parser.parse_args()

    public_url = args.url.rstrip('/')

    # Setup proxy configuration
    proxies = None
    if args.proxy:
        proxies = {
            'http': args.proxy,
            'https': args.proxy
        }

    # Build headers based on auth type
    if args.auth_type == 'jwt':
        headers = {
            'Authorization': f'Bearer {args.auth_token}',
            'Content-Type': 'application/json'
        }
        auth_header = {'Authorization': f'Bearer {args.auth_token}'}
    else:  # identity
        headers = {
            'x-rh-identity': args.auth_token,
            'Content-Type': 'application/json'
        }
        auth_header = {'x-rh-identity': args.auth_token}

    print("=" * 50)
    print("Simple Export Service Test")
    print("=" * 50)
    print(f"Auth type: {args.auth_type}")
    if proxies:
        print(f"Proxy: {args.proxy}")
    print()

    # Step 1: Create export
    print("[1/3] Creating export...")
    timestamp = datetime.utcnow().isoformat() + 'Z'
    expires_at = (datetime.utcnow() + timedelta(days=7)).isoformat() + 'Z'

    payload = {
        "name": f"Simple Test Export - {timestamp}",
        "format": "json",
        "expires_at": expires_at,
        "sources": [
            {
                "application": "urn:redhat:application:inventory",
                "resource": "urn:redhat:application:inventory:export:systems",
                "filters": {
                    }
                }
            ]
    }

    response = None
    try:
        response = requests.post(
            f"{public_url}/exports",
            headers=headers,
            json=payload,
            timeout=30,
            proxies=proxies
        )
        response.raise_for_status()
        data = response.json()
        export_id = data.get('id')

        if not export_id:
            request_id = get_request_id(response)
            msg = f"✗ No export ID in response: {data}"
            if request_id:
                msg += f" [request_id: {request_id}]"
            print(msg)
            sys.exit(1)

        print(f"✓ Created export: {export_id}")
        print()

    except requests.RequestException as e:
        print(e)
        print(response)
        request_id = get_request_id(response)
        print(request_id )
        msg = f"✗ Failed to create export: {e}"
        if hasattr(e, 'response') and e.response is not None:
            request_id = get_request_id(e.response)
            if request_id:
                msg += f" [request_id: {request_id}]"
        print(msg)
        sys.exit(1)

    # Step 2: Poll status
    print(f"[2/3] Polling status (checking every {args.poll_interval} seconds)...")
    status = None

    for attempt in range(args.max_attempts):
        try:
            response = requests.get(
                f"{public_url}/exports/{export_id}/status",
                headers=auth_header,
                timeout=10,
                proxies=proxies
            )
            response.raise_for_status()
            data = response.json()
            status = data.get('status')

            print(f"  [{attempt + 1}/{args.max_attempts}] Status: {status}")

            if status in ["complete", "partial"]:
                print("✓ Export ready for download!")
                break
            elif status == "failed":
                request_id = get_request_id(response)
                print("✗ Export failed!")
                if request_id:
                    print(f"Request ID: {request_id}")
                print(json.dumps(data, indent=2))
                sys.exit(1)

            time.sleep(args.poll_interval)

        except requests.RequestException as e:
            msg = f"  [{attempt + 1}/{args.max_attempts}] Error: {e}"
            if hasattr(e, 'response') and e.response is not None:
                request_id = get_request_id(e.response)
                if request_id:
                    msg += f" [request_id: {request_id}]"
            print(msg)
            time.sleep(args.poll_interval)

    if status not in ["complete", "partial"]:
        print("✗ Timeout waiting for export to complete")
        sys.exit(1)

    print()

    # Step 3: Download export
    print("[3/3] Downloading export...")
    output_file = args.output or f"{export_id}.zip"

    try:
        response = requests.get(
            f"{public_url}/exports/{export_id}",
            headers=auth_header,
            timeout=60,
            stream=True,
            proxies=proxies
        )
        response.raise_for_status()

        with open(output_file, 'wb') as f:
            for chunk in response.iter_content(chunk_size=8192):
                f.write(chunk)

        file_size = Path(output_file).stat().st_size
        print(f"✓ Downloaded to: {output_file} ({file_size:,} bytes)")

    except requests.RequestException as e:
        msg = f"✗ Download failed: {e}"
        if hasattr(e, 'response') and e.response is not None:
            request_id = get_request_id(e.response)
            if request_id:
                msg += f" [request_id: {request_id}]"
        print(msg)
        if Path(output_file).exists():
            Path(output_file).unlink()
        sys.exit(1)

    print()
    print("=" * 50)
    print("Test completed successfully!")
    print(f"Export ID: {export_id}")
    print(f"File: {output_file}")
    print("=" * 50)


if __name__ == '__main__':
    main()

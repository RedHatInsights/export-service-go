#!/usr/bin/env python3

"""
Continuous Export Service Test Script
Creates exports every 20s, checks status every 1s, downloads every 10s
"""

import argparse
import json
import os
import signal
import sys
import threading
import time
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Optional

import requests

# ANSI color codes
class Colors:
    RED = '\033[0;31m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    NC = '\033[0m'  # No Color


@dataclass
class ExportRecord:
    """Track an export's metadata"""
    export_id: str
    created_at: float
    last_status: str = "pending"


class ExportTester:
    """Continuous export service tester"""

    def __init__(
        self,
        public_url: str,
        auth_token: str,
        auth_type: str = "identity",
        create_interval: int = 20,
        status_interval: int = 1,
        download_interval: int = 10,
        log_dir: str = "./test_logs",
        download_dir: str = "./test_downloads",
        proxy: Optional[str] = None
    ):
        self.public_url = public_url.rstrip('/')
        self.auth_token = auth_token
        self.auth_type = auth_type.lower()
        self.create_interval = create_interval
        self.status_interval = status_interval
        self.download_interval = download_interval
        self.log_dir = Path(log_dir)
        self.download_dir = Path(download_dir)

        # Setup proxy configuration
        self.proxies = None
        if proxy:
            self.proxies = {
                'http': proxy,
                'https': proxy
            }

        # Validate auth type
        if self.auth_type not in ["identity", "jwt"]:
            raise ValueError(f"Invalid auth_type '{auth_type}'. Must be 'identity' or 'jwt'")

        # Create directories
        self.log_dir.mkdir(exist_ok=True)
        self.download_dir.mkdir(exist_ok=True)

        # Tracking
        self.tracking_file = self.log_dir / "exports.json"
        self.exports: Dict[str, ExportRecord] = {}
        self.current_export_id: Optional[str] = None  # Track only the most recent export
        self.exports_lock = threading.Lock()
        self.running = True

        # Statistics
        self.stats = {
            'created': 0,
            'downloaded': 0,
            'failed': 0,
            'completed': 0,
        }

        # Load existing exports
        self._load_tracking()

        # Setup signal handlers
        signal.signal(signal.SIGINT, self._signal_handler)
        signal.signal(signal.SIGTERM, self._signal_handler)

    def _get_auth_headers(self) -> dict:
        """Get authentication headers based on auth type"""
        if self.auth_type == "jwt":
            return {
                'Authorization': f'Bearer {self.auth_token}',
                'Content-Type': 'application/json'
            }
        else:  # identity
            return {
                'x-rh-identity': self.auth_token,
                'Content-Type': 'application/json'
            }

    def _log(self, level: str, message: str, color: str = Colors.NC):
        """Log a message with timestamp and color"""
        timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
        print(f"{color}[{timestamp}] [{level}]{Colors.NC} {message}", flush=True)

    def log_info(self, message: str):
        self._log("INFO", message, Colors.BLUE)

    def log_success(self, message: str):
        self._log("SUCCESS", message, Colors.GREEN)

    def log_error(self, message: str, request_id: Optional[str] = None):
        if request_id:
            message = f"{message} [request_id: {request_id}]"
        self._log("ERROR", message, Colors.RED)

    def log_warning(self, message: str):
        self._log("WARNING", message, Colors.YELLOW)

    def _get_request_id(self, response: requests.Response) -> Optional[str]:
        """Extract request ID from response headers"""
        # Try different header variations
        for header in ['x-rh-insights-request-id', 'x-rh-request-id', 'x-request-id']:
            request_id = response.headers.get(header)
            if request_id:
                return request_id
        return None

    def _signal_handler(self, signum, frame):
        """Handle shutdown signals"""
        self.log_warning("Shutting down...")
        self.running = False

    def _load_tracking(self):
        """Load export tracking from file"""
        if self.tracking_file.exists():
            try:
                with open(self.tracking_file, 'r') as f:
                    data = json.load(f)
                    for export_id, record in data.items():
                        self.exports[export_id] = ExportRecord(**record)
                self.log_info(f"Loaded {len(self.exports)} tracked exports")
            except Exception as e:
                self.log_error(f"Failed to load tracking file: {e}")

    def _save_tracking(self):
        """Save export tracking to file"""
        try:
            with self.exports_lock:
                data = {
                    export_id: {
                        'export_id': record.export_id,
                        'created_at': record.created_at,
                        'last_status': record.last_status
                    }
                    for export_id, record in self.exports.items()
                }
            with open(self.tracking_file, 'w') as f:
                json.dump(data, f, indent=2)
        except Exception as e:
            self.log_error(f"Failed to save tracking file: {e}")

    def _create_export_payload(self) -> dict:
        """Generate export request payload"""
        timestamp = datetime.utcnow().isoformat() + 'Z'
        expires_at = (datetime.utcnow() + timedelta(days=7)).isoformat() + 'Z'

        return {
            "name": f"Continuous Test Export - {timestamp}",
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

    def create_export(self):
        """Create a new export"""
        try:
            self.log_info("Creating new export...")

            headers = self._get_auth_headers()
            payload = self._create_export_payload()

            response = requests.post(
                f"{self.public_url}/exports",
                headers=headers,
                json=payload,
                timeout=30,
                proxies=self.proxies
            )

            if response.status_code != 202:
                request_id = self._get_request_id(response)
                self.log_error(f"Failed to create export: HTTP {response.status_code}", request_id)
                self.stats['failed'] += 1
                return

            data = response.json()
            export_id = data.get('id')

            if not export_id:
                request_id = self._get_request_id(response)
                self.log_error(f"No export ID in response: {data}", request_id)
                self.stats['failed'] += 1
                return

            # Track the export
            record = ExportRecord(
                export_id=export_id,
                created_at=time.time(),
                last_status="pending"
            )

            with self.exports_lock:
                # Clear old exports and only keep the new one
                old_export_id = self.current_export_id
                if old_export_id and old_export_id in self.exports:
                    old_status = self.exports[old_export_id].last_status
                    if old_status not in ["complete", "partial"]:
                        self.log_info(f"Abandoning previous export {old_export_id} (status: {old_status})")

                # Set the new export as current
                self.current_export_id = export_id
                self.exports[export_id] = record

            self.stats['created'] += 1
            self.log_success(f"Created export: {export_id}")

            # Save full response
            log_file = self.log_dir / f"{export_id}.json"
            with open(log_file, 'w') as f:
                json.dump(data, f, indent=2)

            self._save_tracking()

        except requests.RequestException as e:
            request_id = None
            if hasattr(e, 'response') and e.response is not None:
                request_id = self._get_request_id(e.response)
            self.log_error(f"Network error creating export: {e}", request_id)
            self.stats['failed'] += 1
        except Exception as e:
            self.log_error(f"Error creating export: {e}")
            self.stats['failed'] += 1

    def check_export_status(self):
        """Check status of the current export only"""
        with self.exports_lock:
            export_id = self.current_export_id

        if not export_id:
            return

        # Only check the current export
        try:
            headers = self._get_auth_headers()
            response = requests.get(
                f"{self.public_url}/exports/{export_id}/status",
                headers=headers,
                timeout=10,
                proxies=self.proxies
            )

            if response.status_code != 200:
                return

            data = response.json()
            current_status = data.get('status', 'error')

            with self.exports_lock:
                record = self.exports.get(export_id)
                if not record:
                    return

                # Status changed
                if current_status != record.last_status:
                    self.log_info(f"Export {export_id}: {record.last_status} → {current_status}")
                    record.last_status = current_status

                    # Update stats
                    if current_status == "complete":
                        self.stats['completed'] += 1
                    elif current_status == "failed":
                        self.stats['failed'] += 1

            # Log with color coding
            if current_status == "complete":
                self.log_success(f"Export {export_id}: {current_status}")
            elif current_status == "failed":
                self.log_error(f"Export {export_id}: {current_status}")
            elif current_status == "partial":
                self.log_warning(f"Export {export_id}: {current_status}")
            else:
                self.log_info(f"Export {export_id}: {current_status}")

        except requests.RequestException:
            pass  # Silently skip network errors during status check
        except Exception as e:
            self.log_error(f"Error checking status for {export_id}: {e}")

    def download_exports(self):
        """Download the current export if completed"""
        with self.exports_lock:
            export_id = self.current_export_id
            record = self.exports.get(export_id) if export_id else None

        if not export_id or not record:
            return

        # Only process the current export
        try:
            # Check if already downloaded
            download_file = self.download_dir / f"{export_id}.zip"
            if download_file.exists():
                return

            # Only download if complete or partial
            if record.last_status not in ["complete", "partial"]:
                return

            self.log_info(f"Downloading export {export_id}...")

            headers = self._get_auth_headers()
            response = requests.get(
                f"{self.public_url}/exports/{export_id}",
                headers=headers,
                timeout=60,
                stream=True,
                proxies=self.proxies
            )

            if response.status_code == 200:
                with open(download_file, 'wb') as f:
                    for chunk in response.iter_content(chunk_size=8192):
                        f.write(chunk)

                file_size = download_file.stat().st_size
                self.stats['downloaded'] += 1
                self.log_success(f"Downloaded export {export_id} ({file_size:,} bytes)")
            else:
                request_id = self._get_request_id(response)
                self.log_error(f"Failed to download export {export_id} (HTTP {response.status_code})", request_id)

        except requests.RequestException as e:
            request_id = None
            if hasattr(e, 'response') and e.response is not None:
                request_id = self._get_request_id(e.response)
            self.log_error(f"Network error downloading {export_id}: {e}", request_id)
        except Exception as e:
            self.log_error(f"Error downloading {export_id}: {e}")

    def cleanup_old_exports(self):
        """Remove old exports from tracking, keeping only the current one"""
        with self.exports_lock:
            if not self.current_export_id:
                return

            # Keep only the current export
            current_export = self.exports.get(self.current_export_id)
            if current_export:
                old_count = len(self.exports) - 1
                self.exports = {self.current_export_id: current_export}

                if old_count > 0:
                    self.log_info(f"Cleaned up {old_count} old exports from tracking")
                    self._save_tracking()

    def _create_loop(self):
        """Background thread: Create exports"""
        while self.running:
            self.create_export()
            for _ in range(self.create_interval):
                if not self.running:
                    break
                time.sleep(1)

    def _status_loop(self):
        """Background thread: Check status"""
        while self.running:
            time.sleep(self.status_interval)
            if self.running:
                self.check_export_status()

    def _download_loop(self):
        """Background thread: Download exports"""
        while self.running:
            time.sleep(self.download_interval)
            if self.running:
                self.download_exports()

    def _cleanup_loop(self):
        """Background thread: Cleanup old exports"""
        while self.running:
            time.sleep(300)  # 5 minutes
            if self.running:
                self.cleanup_old_exports()

    def print_summary(self):
        """Print test run summary"""
        self.log_info("Test run summary:")
        self.log_info(f"  Total exports created: {self.stats['created']}")
        self.log_info(f"  Completed: {self.stats['completed']}")
        self.log_info(f"  Downloaded: {self.stats['downloaded']}")
        self.log_info(f"  Failed: {self.stats['failed']}")
        if self.current_export_id:
            self.log_info(f"  Current export: {self.current_export_id}")
        self.log_info(f"  Log files in: {self.log_dir}")
        self.log_info(f"  Downloads in: {self.download_dir}")

    def run(self):
        """Start the continuous test"""
        self.log_info("Starting continuous export test...")
        self.log_info("Configuration:")
        self.log_info(f"  API URL: {self.public_url}")
        self.log_info(f"  Auth type: {self.auth_type}")
        if self.proxies:
            self.log_info(f"  Proxy: {self.proxies.get('http')}")
        self.log_info(f"  Create interval: {self.create_interval}s")
        self.log_info(f"  Status check interval: {self.status_interval}s")
        self.log_info(f"  Download check interval: {self.download_interval}s")
        self.log_info(f"  Logs: {self.log_dir}")
        self.log_info(f"  Downloads: {self.download_dir}")
        self.log_info("")
        self.log_info("Note: Only the most recent export is monitored. Previous exports are abandoned when a new one is created.")
        self.log_info("")
        self.log_info("Press Ctrl+C to stop")
        self.log_info("----------------------------------------")

        # Start background threads
        threads = [
            threading.Thread(target=self._create_loop, name="Creator", daemon=True),
            threading.Thread(target=self._status_loop, name="Status", daemon=True),
            threading.Thread(target=self._download_loop, name="Downloader", daemon=True),
            threading.Thread(target=self._cleanup_loop, name="Cleanup", daemon=True),
        ]

        for thread in threads:
            thread.start()

        # Wait for shutdown signal
        try:
            while self.running:
                time.sleep(1)
        except KeyboardInterrupt:
            self.running = False

        # Wait for threads to finish
        self.log_info("Waiting for threads to finish...")
        for thread in threads:
            thread.join(timeout=2)

        # Save final state
        self._save_tracking()

        # Print summary
        self.print_summary()


def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(
        description="Continuous Export Service Test Script",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s
  %(prog)s --url http://localhost:8000/api/export/v1
  %(prog)s --create-interval 30 --status-interval 2
        """
    )

    parser.add_argument(
        '--url',
        default=os.environ.get('PUBLIC_URL', 'http://localhost:8000/api/export/v1'),
        help='Export service API URL (default: $PUBLIC_URL or http://localhost:8000/api/export/v1)'
    )

    parser.add_argument(
        '--auth-token',
        default=os.environ.get('AUTH_TOKEN') or os.environ.get('RH_IDENTITY', 'eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiJhY2NvdW50MTIzIiwib3JnX2lkIjoib3JnMTIzIiwidHlwZSI6IlVzZXIiLCJ1c2VyIjp7ImlzX29yZ19hZG1pbiI6dHJ1ZX0sImludGVybmFsIjp7Im9yZ19pZCI6Im9yZzEyMyJ9fX0K'),
        help='Authentication token - either x-rh-identity value or JWT token (default: $AUTH_TOKEN, $RH_IDENTITY, or test identity)'
    )

    parser.add_argument(
        '--auth-type',
        choices=['identity', 'jwt'],
        default=os.environ.get('AUTH_TYPE', 'identity'),
        help='Authentication type: "identity" for x-rh-identity header, "jwt" for Authorization: Bearer (default: $AUTH_TYPE or identity)'
    )

    parser.add_argument(
        '--create-interval',
        type=int,
        default=20,
        help='Seconds between creating exports (default: 20)'
    )

    parser.add_argument(
        '--status-interval',
        type=int,
        default=1,
        help='Seconds between status checks (default: 1)'
    )

    parser.add_argument(
        '--download-interval',
        type=int,
        default=10,
        help='Seconds between download attempts (default: 10)'
    )

    parser.add_argument(
        '--log-dir',
        default='./test_logs',
        help='Directory for log files (default: ./test_logs)'
    )

    parser.add_argument(
        '--download-dir',
        default='./test_downloads',
        help='Directory for downloads (default: ./test_downloads)'
    )

    parser.add_argument(
        '--proxy',
        default=os.environ.get('HTTP_PROXY') or os.environ.get('HTTPS_PROXY'),
        help='HTTP/HTTPS proxy URL (e.g., http://proxy.example.com:3128) (default: $HTTP_PROXY or $HTTPS_PROXY)'
    )

    args = parser.parse_args()

    # Create and run tester
    tester = ExportTester(
        public_url=args.url,
        auth_token=args.auth_token,
        auth_type=args.auth_type,
        create_interval=args.create_interval,
        status_interval=args.status_interval,
        download_interval=args.download_interval,
        log_dir=args.log_dir,
        download_dir=args.download_dir,
        proxy=args.proxy
    )

    tester.run()


if __name__ == '__main__':
    main()

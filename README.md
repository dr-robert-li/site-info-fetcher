# Site Information Fetcher
### Create by Robert Li
### Version 1.0

This Go program fetches various pieces of information about WordPress sites from a list of URLs provided in a CSV file. It performs multiple Time to First Byte (TTFB) tests, checks for specific headers, and writes the results to a new CSV file.

***There is no warranty, support or maintenance provided for this program. Use at your own risk.***

## Features

- Fetches site information including PHP version, MySQL version, WordPress version, caching status, cache control, web server, web server version, SSL validity, and `X-Powered-By` header.
- Performs three TTFB tests and calculates the average TTFB.
- Sorts TTFB tests from longest to shortest latency.
- Checks if the SSL certificate is valid.
- Fetches supported versions of PHP, MySQL, WordPress, and web servers from the endoflife.date API.
- Determines the support status of PHP, MySQL, WordPress, and web server versions.
- Writes the results to a new CSV file with a timestamp in the filename.

## Prerequisites

- Go 1.16 or later

## Installation

1. Clone the repository.

2. Build the program:

```sh
go build -o site-info-fetcher main.go
```

## Usage

1. Prepare a CSV file (e.g., urls.csv) with the URLs you want to analyze. Ensure the URLs are in a single column.

2. Run the program:

```sh
./site-info-fetcher
```

3. Follow the prompts:

```sh
Enter the path to your CSV file (e.g., urls.csv).
Enter the column number containing the URLs (starting from 0).
```

## View the output:

The program will fetch the site information for each URL, print the three TTFB tests (sorted from longest to shortest) and the average TTFB in milliseconds (ms) in the terminal. The results will be written to a new CSV file with a timestamp in the filename, e.g., site_info_20230101_123456.csv, in the same directory.

## License

This project is licensed under the MIT License. See the LICENSE file for details.

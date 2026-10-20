#!/usr/bin/env ruby
# frozen_string_literal: true

# assess_fines.rb - accrue overdue fines for items still out.
# Replaced fines.pl in 2019 (CIRC-1102). The arithmetic lives in the stored
# procedure sp_assess_fines so the desk (FineCalculator.java) and this job agree.

require 'optparse'
require 'yaml'
require 'pg'
require 'logger'

module Riverbend
  # @gloss implements .NightlyFineJob conf=0.85
  class FineAssessor
    def initialize(conn, config, logger)
      @conn = conn
      @config = config
      @logger = logger
    end

    def run(as_of, dry_run: false)
      @conn.transaction do |tx|
        result = tx.exec_params('SELECT * FROM sp_assess_fines($1::date)', [as_of])
        row = result.first
        @logger.info("#{as_of} assess_fines: #{row['loans_charged']} loans, $#{row['total_charged']} charged")
        raise PG::Error, 'dry run' if dry_run
      end
    rescue PG::Error => e
      raise unless dry_run && e.message == 'dry run'

      @logger.info('dry run: rolled back')
    end
  end
end

if $PROGRAM_NAME == __FILE__
  options = { date: Time.now.strftime('%Y-%m-%d'), dry_run: false }
  OptionParser.new do |o|
    o.on('--date DATE') { |d| options[:date] = d }
    o.on('--dry-run') { options[:dry_run] = true }
  end.parse!

  config_dir = ENV.fetch('CIRC_CONFIG_DIR', '/etc/circ')
  config = YAML.safe_load(File.read(File.join(config_dir, 'batch.yaml')))
  conn = PG.connect(host: ENV.fetch('CIRC_DB_HOST', 'localhost'), dbname: ENV.fetch('CIRC_DB_NAME', 'circ'),
                    user: ENV.fetch('CIRC_DB_USER', 'circ_batch'), password: ENV['CIRC_DB_PASSWORD'])
  Riverbend::FineAssessor.new(conn, config, Logger.new($stdout)).run(options[:date], dry_run: options[:dry_run])
end
